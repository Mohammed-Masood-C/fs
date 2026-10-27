// Package cli implements the fs command line: serving a folder in the
// foreground or background, and managing the background services.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"fs/internal/certs"
	"fs/internal/config"
	"fs/internal/fileserver"
	"fs/internal/middleware"
	"fs/internal/server"
	"fs/internal/services"
	"fs/internal/version"
)

// Process exit codes.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

// serviceCommand is the hidden command a background service runs as. Its
// settings come from the service record, so no password is ever visible on
// a command line.
const serviceCommand = "__service"

// Run runs fs with the given arguments (without the program name) and returns
// the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "start":
			return cmdServe(args[1:], true, stdout, stderr)
		case "stop":
			return cmdStop(args[1:], stdout, stderr)
		case "ps":
			return cmdPs(args[1:], stdout, stderr)
		case "logs":
			return cmdLogs(args[1:], stdout, stderr)
		case serviceCommand:
			return cmdService(args[1:], stdout, stderr)
		}
	}
	return cmdServe(args, false, stdout, stderr)
}

// cmdServe serves a folder in the foreground, or in the background with -d
// or "fs start".
func cmdServe(args []string, background bool, stdout, stderr io.Writer) int {
	opts, err := parseFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "fs: %v\nRun 'fs --help' for usage.\n", err)
		return exitUsage
	}
	switch {
	case opts.help:
		fmt.Fprint(stdout, usage)
		return exitOK
	case opts.version:
		fmt.Fprintf(stdout, "fs %s\n", version.String())
		return exitOK
	case opts.setDefault != "":
		return setDefaultPath(opts.setDefault, stdout, stderr)
	}

	dir, code := folderToServe(opts.folder, stderr)
	if code != exitOK {
		return code
	}
	rec := &services.Record{
		State:   services.StateStarting,
		Folder:  dir,
		Host:    opts.host,
		Port:    opts.port,
		NoIndex: opts.noIndex,
		Verbose: opts.verbose,
		Started: time.Now(),
	}
	rec.User, rec.Password, _ = strings.Cut(opts.auth, ":")

	if background || opts.detach {
		rec.Background = true
		return startBackground(rec, stdout, stderr)
	}

	rec.PID = os.Getpid()
	rec.Created = services.SelfCreated()
	if err := services.Register(rec); err != nil {
		fmt.Fprintf(stderr, "fs: register service: %v\n", err)
		return exitError
	}
	defer services.Remove(rec.ID)

	if err := serve(rec, newLogger(stderr, rec.Verbose), stdout); err != nil {
		fmt.Fprintf(stderr, "fs: %v\n", err)
		return exitError
	}
	return exitOK
}

// cmdService is the body of a background service: it serves the folder
// described by its record, with output going to the service's log.
func cmdService(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		return exitUsage
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return exitUsage
	}
	rec, err := services.Load(id)
	if err != nil {
		fmt.Fprintf(stderr, "fs: load service %d: %v\n", id, err)
		return exitError
	}
	defer services.Remove(rec.ID)

	rec.PID = os.Getpid()
	rec.Created = services.SelfCreated()
	if err := services.Save(&rec); err != nil {
		fmt.Fprintf(stderr, "fs: update service %d: %v\n", id, err)
		return exitError
	}
	if err := serve(&rec, newLogger(stderr, rec.Verbose), stdout); err != nil {
		fmt.Fprintf(stderr, "fs: %v\n", err)
		return exitError
	}
	return exitOK
}

// serve serves the folder of rec until it is interrupted (Ctrl+C) or asked to
// stop by "fs stop", then shuts down gracefully.
func serve(rec *services.Record, logger *slog.Logger, stdout io.Writer) error {
	// Every file access goes through root, which refuses paths and symbolic
	// links that would escape the folder.
	root, err := os.OpenRoot(rec.Folder)
	if err != nil {
		return fmt.Errorf("open folder: %w", err)
	}
	defer root.Close()

	files, err := fileserver.New(root, fileserver.Options{
		DisableIndex: rec.NoIndex,
		Logger:       logger,
	})
	if err != nil {
		return err
	}

	cert, certInfo, err := certs.Load(certHosts(rec.Host))
	if err != nil {
		return err
	}

	srv, err := server.New(server.Config{
		Addr:        net.JoinHostPort(rec.Host, strconv.Itoa(rec.Port)),
		Certificate: cert,
	}, buildHandler(files, rec, logger), logger)
	if err != nil {
		return err
	}

	// The actual port matters when --port 0 picked a free one.
	if tcp, ok := srv.Addr().(*net.TCPAddr); ok {
		rec.Port = tcp.Port
	}
	rec.State = services.StateRunning
	if err := services.Save(rec); err != nil {
		logger.Warn("could not update the service record", "err", err)
	}

	printBanner(stdout, rec, srv.Addr(), certInfo)
	logger.Info("server started", "id", rec.ID, "addr", srv.Addr().String(), "folder", rec.Folder)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Restore the default signal behaviour as soon as shutdown begins, so that
	// a second Ctrl+C terminates the process immediately.
	context.AfterFunc(ctx, stop)
	go services.WatchStop(ctx, rec.ID, stop)

	return srv.Run(ctx)
}

// buildHandler wraps the file server in the middleware stack. The first
// middleware in the stack is the outermost one.
func buildHandler(files http.Handler, rec *services.Record, logger *slog.Logger) http.Handler {
	var stack []middleware.Middleware
	if rec.Verbose {
		stack = append(stack, middleware.AccessLog(logger))
	}
	stack = append(stack,
		middleware.Recover(logger),
		middleware.SecurityHeaders,
		middleware.CORS,
		middleware.AllowMethods(http.MethodGet, http.MethodHead),
	)
	if rec.User != "" {
		stack = append(stack, middleware.BasicAuth(rec.User, rec.Password, "fs"))
	}
	return middleware.Chain(files, stack...)
}

// newLogger logs to w: only warnings and errors by default, everything
// (including one line per request) when verbose.
func newLogger(w io.Writer, verbose bool) *slog.Logger {
	level := slog.LevelWarn
	if verbose {
		level = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
}

// setDefaultPath stores folder as the one served when fs runs without a
// folder argument.
func setDefaultPath(folder string, stdout, stderr io.Writer) int {
	dir, err := resolveFolder(folder)
	if err != nil {
		fmt.Fprintf(stderr, "fs: %v\n", err)
		return exitError
	}
	if err := config.SetDefaultPath(dir); err != nil {
		fmt.Fprintf(stderr, "fs: save default folder: %v\n", err)
		return exitError
	}
	fmt.Fprintf(stdout, "Default folder set to %s\nRun fs without a folder to serve it.\n", dir)
	return exitOK
}

// folderToServe returns the absolute path of the folder given on the command
// line or, without one, of the default folder. On failure it reports the
// problem and returns a non-zero exit code.
func folderToServe(folder string, stderr io.Writer) (string, int) {
	if folder != "" {
		dir, err := resolveFolder(folder)
		if err != nil {
			fmt.Fprintf(stderr, "fs: %v\n", err)
			return "", exitError
		}
		return dir, exitOK
	}

	def, err := config.DefaultPath()
	if err != nil {
		fmt.Fprintf(stderr, "fs: read default folder: %v\n", err)
		return "", exitError
	}
	if def == "" {
		fmt.Fprint(stderr, "fs: missing the folder to serve\n"+
			"Pass one (fs <folder>), or set a default with: fs --set-default-path <folder>\n")
		return "", exitUsage
	}
	dir, err := resolveFolder(def)
	if err != nil {
		fmt.Fprintf(stderr, "fs: default folder: %v\nChange it with: fs --set-default-path <folder>\n", err)
		return "", exitError
	}
	return dir, exitOK
}

// resolveFolder returns the absolute path of folder after checking that it is
// an existing directory.
func resolveFolder(folder string) (string, error) {
	dir, err := filepath.Abs(folder)
	if err != nil {
		return "", fmt.Errorf("resolve folder %q: %w", folder, err)
	}
	info, err := os.Stat(dir)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "", fmt.Errorf("folder %s does not exist", dir)
	case err != nil:
		return "", fmt.Errorf("cannot access folder: %w", err)
	case !info.IsDir():
		return "", fmt.Errorf("%s is not a folder", dir)
	}
	return dir, nil
}
