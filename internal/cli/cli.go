// Package cli implements the fs command line: it parses options, opens the
// served folder and runs the HTTP server until it is interrupted.
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

	"fs/internal/certs"
	"fs/internal/fileserver"
	"fs/internal/middleware"
	"fs/internal/server"
	"fs/internal/version"
)

// Process exit codes.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

// Run runs fs with the given arguments (without the program name) and returns
// the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
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
	}

	// Quiet by default: only warnings and errors, besides the startup banner.
	level := slog.LevelWarn
	if opts.verbose {
		level = slog.LevelInfo
	}
	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level}))
	if err := serve(opts, logger, stdout); err != nil {
		fmt.Fprintf(stderr, "fs: %v\n", err)
		return exitError
	}
	return exitOK
}

// serve opens the folder, starts the server and blocks until SIGINT or
// SIGTERM, after which the server shuts down gracefully.
func serve(opts *options, logger *slog.Logger, stdout io.Writer) error {
	dir, err := resolveFolder(opts.folder)
	if err != nil {
		return err
	}

	// Every file access goes through root, which refuses paths and symbolic
	// links that would escape the folder.
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("open folder: %w", err)
	}
	defer root.Close()

	files, err := fileserver.New(root, fileserver.Options{
		DisableIndex: opts.noIndex,
		Logger:       logger,
	})
	if err != nil {
		return err
	}

	cert, certInfo, err := certs.Load(certHosts(opts.host))
	if err != nil {
		return err
	}

	srv, err := server.New(server.Config{
		Addr:        net.JoinHostPort(opts.host, strconv.Itoa(opts.port)),
		Certificate: cert,
	}, buildHandler(files, opts, logger), logger)
	if err != nil {
		return err
	}

	printBanner(stdout, dir, srv.Addr(), certInfo)
	logger.Info("server started", "addr", srv.Addr().String(), "folder", dir)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Restore the default signal behaviour as soon as shutdown begins, so that
	// a second Ctrl+C terminates the process immediately.
	context.AfterFunc(ctx, stop)

	return srv.Run(ctx)
}

// buildHandler wraps the file server in the middleware stack. The first
// middleware in the stack is the outermost one.
func buildHandler(files http.Handler, opts *options, logger *slog.Logger) http.Handler {
	var stack []middleware.Middleware
	if opts.verbose {
		stack = append(stack, middleware.AccessLog(logger))
	}
	stack = append(stack,
		middleware.Recover(logger),
		middleware.SecurityHeaders,
		middleware.CORS,
		middleware.AllowMethods(http.MethodGet, http.MethodHead),
	)
	if user, pass, ok := strings.Cut(opts.auth, ":"); ok {
		stack = append(stack, middleware.BasicAuth(user, pass, "fs"))
	}
	return middleware.Chain(files, stack...)
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
