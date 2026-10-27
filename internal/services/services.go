// Package services keeps track of running fs instances ("services") so they
// can be listed, stopped and inspected from any terminal.
//
// Each instance has a record in %AppData%\fs\services\<id>.json and, when it
// runs in the background, a log in <id>.log. Another process asks an instance
// to stop by creating <id>.stop, which the instance checks for twice a second.
package services

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"fs/internal/config"
)

// Service states.
const (
	StateStarting = "starting"
	StateRunning  = "running"
)

const (
	// stopTimeout is how long Stop waits for a graceful shutdown before the
	// process is killed. It exceeds the server's own shutdown grace period.
	stopTimeout = 15 * time.Second
	// orphanAfter is how long a record may stay without a process before it
	// is considered left over from a launch that failed.
	orphanAfter = time.Minute
)

// Record describes one fs instance.
type Record struct {
	ID         int       `json:"id"`
	PID        int       `json:"pid"`
	Created    int64     `json:"created"` // process creation time; guards against PID reuse
	State      string    `json:"state"`
	Background bool      `json:"background"`
	Folder     string    `json:"folder"`
	Host       string    `json:"host"`
	Port       int       `json:"port"`
	User       string    `json:"user,omitempty"`
	Password   string    `json:"password,omitempty"`
	NoIndex    bool      `json:"no_index"`
	Verbose    bool      `json:"verbose"`
	Started    time.Time `json:"started"`
}

func dir() (string, error) {
	d, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "services"), nil
}

func file(id int, ext string) (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, strconv.Itoa(id)+ext), nil
}

// LogFile returns the path of the log of the background service id.
func LogFile(id int) (string, error) { return file(id, ".log") }

// Register gives rec the next free id and stores it. IDs count up from the
// highest running service, so they start at 1 again once none is running.
func Register(rec *Record) error {
	d, err := dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return err
	}
	running, err := List()
	if err != nil {
		return err
	}
	id := 1
	for _, r := range running {
		id = max(id, r.ID+1)
	}

	for ; ; id++ {
		p := filepath.Join(d, strconv.Itoa(id)+".json")
		// O_EXCL makes the reservation atomic, even when two instances start
		// at the same moment.
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return err
		}
		f.Close()
		rec.ID = id
		_ = os.Remove(filepath.Join(d, strconv.Itoa(id)+".stop")) // left over from an earlier run
		return Save(rec)
	}
}

// Save stores rec, replacing its previous version atomically.
func Save(rec *Record) error {
	p, err := file(rec.ID, ".json")
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	// Windows refuses to replace a file while another process is reading it
	// (e.g. a concurrent "fs ps"), so retry briefly.
	for attempt := 0; ; attempt++ {
		err = os.Rename(tmp, p)
		if err == nil || attempt == 20 {
			return err
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// Load returns the record of service id.
func Load(id int) (Record, error) {
	var rec Record
	p, err := file(id, ".json")
	if err != nil {
		return rec, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return rec, err
	}
	err = json.Unmarshal(data, &rec)
	return rec, err
}

// List returns the services that are running or starting, ordered by id.
//
// It also cleans up after services that ended without "fs stop", e.g.
// because Windows was shut down: their records and logs are deleted.
func List() ([]Record, error) {
	d, err := dir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(d)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var recs []Record
	live := map[int]bool{}
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok {
			continue
		}
		id, err := strconv.Atoi(name)
		if err != nil {
			continue
		}
		rec, err := Load(id)
		if err != nil {
			live[id] = true // being written right now
			continue
		}
		if !alive(rec, e) {
			Remove(id)
			continue
		}
		live[id] = true
		recs = append(recs, rec)
	}

	// Delete logs and other files that no live service owns. Temporary files
	// are only deleted once old, as a Save may be in progress.
	for _, e := range entries {
		prefix, ext, _ := strings.Cut(e.Name(), ".")
		id, err := strconv.Atoi(prefix)
		if err != nil || live[id] || ext == "json" {
			continue
		}
		if strings.HasSuffix(ext, "tmp") {
			if info, err := e.Info(); err != nil || time.Since(info.ModTime()) < orphanAfter {
				continue
			}
		}
		_ = os.Remove(filepath.Join(d, e.Name()))
	}

	slices.SortFunc(recs, func(a, b Record) int { return cmp.Compare(a.ID, b.ID) })
	return recs, nil
}

// Running reports whether service id is running or starting.
func Running(id int) bool {
	rec, err := Load(id)
	return err == nil && (rec.PID == 0 || ProcessAlive(rec.PID, rec.Created))
}

// alive reports whether rec belongs to a live process. A record without a
// process yet is a launch in progress, unless it is old.
func alive(rec Record, e fs.DirEntry) bool {
	if rec.PID != 0 {
		return ProcessAlive(rec.PID, rec.Created)
	}
	info, err := e.Info()
	return err == nil && time.Since(info.ModTime()) < orphanAfter
}

// Remove deletes the record and log of service id. A log that is still open
// (by the service itself) stays until List cleans it up later.
func Remove(id int) {
	for _, ext := range []string{".json", ".stop", ".log"} {
		if p, err := file(id, ext); err == nil {
			_ = os.Remove(p)
		}
	}
}

// Stop asks service rec to shut down gracefully and waits for it, killing
// the process if it does not exit in time. It reports whether it was killed.
func Stop(rec Record) (killed bool, err error) {
	defer Remove(rec.ID)
	p, err := file(rec.ID, ".stop")
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		return false, err
	}
	if rec.PID == 0 || WaitExit(rec.PID, rec.Created, stopTimeout) {
		return false, nil
	}
	if err := Kill(rec.PID, rec.Created); err != nil {
		return true, err
	}
	// Termination is asynchronous; wait for it so the log can be deleted.
	WaitExit(rec.PID, rec.Created, 2*time.Second)
	return true, nil
}

// WatchStop calls stop when another process asks service id to stop. It
// returns when ctx is done.
func WatchStop(ctx context.Context, id int, stop func()) {
	p, err := file(id, ".stop")
	if err != nil {
		return
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := os.Stat(p); err == nil {
				stop()
				return
			}
		}
	}
}
