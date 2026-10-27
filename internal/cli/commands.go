package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"fs/internal/services"
)

// Process creation flags that package syscall does not export.
const (
	detachedProcess        = 0x00000008 // no console: survives closing the terminal
	createBreakawayFromJob = 0x01000000 // leave the terminal's job object, if allowed
)

// startTimeout is how long "fs start" waits for a service to be listening.
const startTimeout = 20 * time.Second

// startBackground registers rec, launches a detached copy of fs to serve it,
// and waits until it is listening, so that errors are reported right away.
func startBackground(rec *services.Record, stdout, stderr io.Writer) int {
	if err := services.Register(rec); err != nil {
		fmt.Fprintf(stderr, "fs: register service: %v\n", err)
		return exitError
	}
	fail := func(format string, args ...any) int {
		services.Remove(rec.ID)
		fmt.Fprintf(stderr, "fs: "+format+"\n", args...)
		return exitError
	}

	logPath, err := services.LogFile(rec.ID)
	if err != nil {
		return fail("log file: %v", err)
	}
	logFile, err := os.Create(logPath) // replaces the log of an earlier service with this id
	if err != nil {
		return fail("create log file: %v", err)
	}
	cmd, err := spawnService(rec.ID, logFile)
	logFile.Close()
	if err != nil {
		return fail("start background service: %v", err)
	}

	p := newPainter(stdout)
	exited := make(chan struct{})
	go func() {
		cmd.Wait()
		close(exited)
	}()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(startTimeout)

	for {
		select {
		case <-exited:
			pe := newPainter(stderr)
			fmt.Fprintf(stderr, "\n  %s the service could not start:\n\n", pe.bold(pe.red("Error:")))
			printLogTail(stderr, logPath)
			fmt.Fprintln(stderr)
			services.Remove(rec.ID)
			return exitError
		case <-deadline:
			fmt.Fprintf(stdout, "\n  %s fs #%d is still starting. Check on it with: fs logs %d\n\n",
				p.yellow("Waiting:"), rec.ID, rec.ID)
			return exitOK
		case <-ticker.C:
			cur, err := services.Load(rec.ID)
			if err != nil || cur.State != services.StateRunning {
				continue
			}
			fmt.Fprintf(stdout, "\n  %s %s\n\n", p.bold(p.green("Started")), p.bold(fmt.Sprintf("fs #%d", cur.ID))+" in the background")
			fmt.Fprintf(stdout, "  %s %s\n\n", p.green("→"), p.cyan(serviceURL(cur)))
			fmt.Fprintf(stdout, "    %s %s\n", p.dim("Folder "), cur.Folder)
			logs := fmt.Sprintf("fs logs %d", cur.ID)
			if !cur.Verbose {
				logs += p.dim("   (warnings and errors only; start with --verbose to log requests)")
			}
			fmt.Fprintf(stdout, "    %s %s\n", p.dim("Logs   "), logs)
			fmt.Fprintf(stdout, "    %s fs stop %d\n\n", p.dim("Stop   "), cur.ID)
			return exitOK
		}
	}
}

// spawnService starts "fs __service <id>" without a console window, detached
// from the terminal so that it keeps running after the terminal is closed.
func spawnService(id int, logFile *os.File) (*exec.Cmd, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	base := uint32(detachedProcess | syscall.CREATE_NEW_PROCESS_GROUP)
	// Breaking away from the terminal's job object keeps the service alive
	// when the terminal closes, but some terminals do not allow it.
	for _, flags := range []uint32{base | createBreakawayFromJob, base} {
		cmd := exec.Command(exe, serviceCommand, strconv.Itoa(id))
		cmd.Dir = filepath.Dir(exe) // don't keep the caller's folder in use
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags, HideWindow: true}
		if err = cmd.Start(); err == nil {
			return cmd, nil
		}
	}
	return nil, err
}

// printLogTail prints the last lines of a log, indented.
func printLogTail(w io.Writer, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) > 15 {
		lines = lines[len(lines)-15:]
	}
	for _, line := range lines {
		fmt.Fprintf(w, "    %s\n", strings.TrimRight(line, "\r"))
	}
}

// cmdStop stops the given services, or all of them.
func cmdStop(args []string, stdout, stderr io.Writer) int {
	running, err := services.List()
	if err != nil {
		fmt.Fprintf(stderr, "fs: list services: %v\n", err)
		return exitError
	}

	targets := running
	if len(args) > 0 {
		targets = nil
		for _, arg := range args {
			id, err := parseID(arg)
			if err != nil {
				fmt.Fprintf(stderr, "fs: %v\n", err)
				return exitUsage
			}
			rec, ok := findService(running, id)
			if !ok {
				fmt.Fprintf(stderr, "fs: no running service with id %d (see fs ps)\n", id)
				return exitError
			}
			targets = append(targets, rec)
		}
	}
	p := newPainter(stdout)
	if len(targets) == 0 {
		fmt.Fprintf(stdout, "\n  No services are running.\n\n")
		return exitOK
	}

	fmt.Fprintln(stdout)
	code := exitOK
	for _, rec := range targets {
		killed, err := services.Stop(rec)
		id := p.bold(fmt.Sprintf("#%d", rec.ID))
		switch {
		case err != nil:
			fmt.Fprintf(stdout, "  %s %s  %s\n", p.bold(p.red("Failed ")), id, err)
			code = exitError
		case killed:
			fmt.Fprintf(stdout, "  %s %s  %s  %s\n", p.bold(p.yellow("Killed ")), id, rec.Folder,
				p.dim("(did not stop in time)"))
		default:
			fmt.Fprintf(stdout, "  %s %s  %s\n", p.bold(p.green("Stopped")), id, rec.Folder)
		}
	}
	fmt.Fprintln(stdout)
	return code
}

// cmdPs prints the running services as a table, like "docker ps".
func cmdPs(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		fmt.Fprintln(stderr, "fs: ps takes no arguments")
		return exitUsage
	}
	running, err := services.List()
	if err != nil {
		fmt.Fprintf(stderr, "fs: list services: %v\n", err)
		return exitError
	}
	p := newPainter(stdout)
	if len(running) == 0 {
		fmt.Fprintf(stdout, "\n  No services are running.  %s\n\n", p.dim("Start one with: fs start <folder>"))
		return exitOK
	}

	none := cell{text: "-", paint: p.dim}
	rows := make([][]cell, 0, len(running))
	for _, rec := range running {
		url := none
		status := cell{text: "Starting", paint: p.yellow}
		if rec.State == services.StateRunning {
			url = cell{text: serviceURL(rec), paint: p.cyan}
			status = cell{text: "Up " + uptime(rec.Started), paint: p.green}
		}
		login := none
		if rec.User != "" {
			login = cell{text: rec.User + ":" + rec.Password}
		}
		index := cell{text: "on"}
		if rec.NoIndex {
			index = cell{text: "off", paint: p.dim}
		}
		mode := cell{text: "foreground"}
		if rec.Background {
			mode = cell{text: "background"}
		}
		rows = append(rows, []cell{
			{text: strconv.Itoa(rec.ID), paint: p.bold},
			{text: rec.Folder},
			url,
			login,
			index,
			mode,
			{text: strconv.Itoa(rec.PID), paint: p.dim},
			status,
		})
	}

	fmt.Fprintln(stdout)
	printTable(stdout, p, []string{"ID", "FOLDER", "URL", "LOGIN", "INDEX", "MODE", "PID", "STATUS"}, rows)
	noun := "services"
	if len(running) == 1 {
		noun = "service"
	}
	fmt.Fprintf(stdout, "\n  %s\n\n", p.dim(fmt.Sprintf("%d %s running", len(running), noun)))
	return exitOK
}

// cmdLogs prints the log of a background service, and with -f keeps
// printing what is added until the service stops or Ctrl+C is pressed.
func cmdLogs(args []string, stdout, stderr io.Writer) int {
	var follow bool
	set := flag.NewFlagSet("logs", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.BoolVar(&follow, "f", false, "")
	set.BoolVar(&follow, "follow", false, "")
	positional, err := parseInterleaved(set, args)
	if err == nil && len(positional) != 1 {
		err = errors.New("usage: fs logs [-f] <id>")
	}
	if err != nil {
		fmt.Fprintf(stderr, "fs: %v\n", err)
		return exitUsage
	}
	id, err := parseID(positional[0])
	if err != nil {
		fmt.Fprintf(stderr, "fs: %v\n", err)
		return exitUsage
	}

	// Listing first also deletes the logs of services that are gone.
	running, err := services.List()
	if err != nil {
		fmt.Fprintf(stderr, "fs: list services: %v\n", err)
		return exitError
	}
	rec, ok := findService(running, id)
	switch {
	case !ok:
		fmt.Fprintf(stderr, "fs: no running service with id %d (see fs ps)\n", id)
		return exitError
	case !rec.Background:
		fmt.Fprintf(stderr, "fs: #%d runs in the foreground; its output is in the terminal it was started from\n", id)
		return exitError
	}

	path, err := services.LogFile(id)
	if err != nil {
		fmt.Fprintf(stderr, "fs: %v\n", err)
		return exitError
	}
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "fs: open log: %v\n", err)
		return exitError
	}
	defer f.Close()

	out := bufio.NewWriter(stdout)
	io.Copy(out, f)
	out.Flush()
	if !follow {
		return exitOK
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return exitOK
		case <-ticker.C:
			n, _ := io.Copy(stdout, f)
			// Stop following once the service has ended, whether through
			// "fs stop" or by being terminated.
			if n == 0 && !services.Running(id) {
				fmt.Fprintf(stdout, "-- fs #%d has stopped --\n", id)
				return exitOK
			}
		}
	}
}

// parseID parses a service id such as "2" or ":2".
func parseID(s string) (int, error) {
	id, err := strconv.Atoi(strings.TrimLeft(s, ":#"))
	if err != nil || id < 1 {
		return 0, fmt.Errorf("invalid service id %q (see fs ps)", s)
	}
	return id, nil
}

func findService(recs []services.Record, id int) (services.Record, bool) {
	for _, rec := range recs {
		if rec.ID == id {
			return rec, true
		}
	}
	return services.Record{}, false
}

// serviceURL is the local URL of a service.
func serviceURL(rec services.Record) string {
	host := rec.Host
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "localhost"
	}
	return siteURL(host, rec.Port)
}

// uptime formats how long ago t was, e.g. "45s", "12m", "3h05m", "2d04h".
func uptime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd%02dh", int(d.Hours())/24, int(d.Hours())%24)
	}
}
