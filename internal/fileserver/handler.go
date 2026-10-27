// Package fileserver serves the contents of a single folder over HTTP and
// renders a file explorer for its sub-folders.
package fileserver

import (
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Options configures a Handler.
type Options struct {
	DisableIndex bool         // never serve index.html in place of the explorer
	Logger       *slog.Logger // defaults to slog.Default()
}

// cacheControl makes browsers revalidate files on every use, which is cheap
// (304 Not Modified) and means changes on disk show up immediately.
const cacheControl = "no-cache"

// indexFiles are served instead of the explorer when a folder contains one.
var indexFiles = []string{"index.html", "index.htm"}

// Handler serves files from an os.Root. All file system access goes through
// the root, so no request can reach a file outside the served folder, not
// even through symbolic links.
type Handler struct {
	root      *os.Root
	opts      Options
	logger    *slog.Logger
	rootLabel string
	tmpl      *templates
}

// New returns a Handler that serves the folder opened as root.
func New(root *os.Root, opts Options) (*Handler, error) {
	tmpl, err := loadTemplates()
	if err != nil {
		return nil, err
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		root:      root,
		opts:      opts,
		logger:    logger,
		rootLabel: rootLabel(root.Name()),
		tmpl:      tmpl,
	}, nil
}

// ServeHTTP maps the URL path onto the folder and serves the file, the
// folder's index file or the explorer.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	urlPath := r.URL.Path
	if urlPath == "" { // absolute-form request such as "GET http://host"
		urlPath = "/"
	}
	if !strings.HasPrefix(urlPath, "/") || !validURLPath(urlPath) {
		h.serveError(w, r, http.StatusBadRequest)
		return
	}
	// Serve every resource under one canonical URL.
	if clean := cleanURLPath(urlPath); clean != urlPath {
		redirect(w, r, clean)
		return
	}

	rel := strings.Trim(urlPath, "/")
	name := "."
	if rel != "" {
		var err error
		if name, err = filepath.Localize(rel); err != nil {
			h.serveError(w, r, http.StatusNotFound)
			return
		}
	}

	info, err := h.root.Stat(name)
	if err != nil {
		h.serveFSError(w, r, err)
		return
	}

	switch {
	case info.IsDir():
		// Folders end in a slash so relative links inside them resolve correctly.
		if !strings.HasSuffix(urlPath, "/") {
			redirect(w, r, urlPath+"/")
			return
		}
		h.serveDir(w, r, name, urlPath)
	case info.Mode().IsRegular():
		if strings.HasSuffix(urlPath, "/") {
			redirect(w, r, strings.TrimSuffix(urlPath, "/"))
			return
		}
		h.serveFile(w, r, name)
	default:
		// Devices, sockets and named pipes are never served: opening a named
		// pipe would block until something writes to it.
		h.serveError(w, r, http.StatusNotFound)
	}
}

// serveDir serves the folder's index file if it has one, and the explorer
// otherwise.
func (h *Handler) serveDir(w http.ResponseWriter, r *http.Request, name, urlPath string) {
	switch q := r.URL.Query(); {
	case q.Has("download"):
		h.serveZip(w, r, name)
		return
	case q.Has("size"):
		h.serveDirSize(w, r, name)
		return
	}
	if !h.opts.DisableIndex {
		for _, index := range indexFiles {
			p := filepath.Join(name, index)
			info, err := h.root.Stat(p)
			if err == nil && info.Mode().IsRegular() {
				h.serveFile(w, r, p)
				return
			}
		}
	}
	h.serveListing(w, r, name, urlPath)
}

// serveFSError answers with the status that corresponds to a file system
// error. Unexpected errors are logged.
func (h *Handler) serveFSError(w http.ResponseWriter, r *http.Request, err error) {
	status := statusForError(err)
	if status == http.StatusInternalServerError {
		h.logger.Error("file system error", "path", r.URL.Path, "err", err)
	}
	h.serveError(w, r, status)
}

// redirect permanently redirects to the URL path p, keeping the query string.
// Building the target with url.URL escapes it, so a path such as "/\evil.com"
// can never turn into a redirect to another host.
func redirect(w http.ResponseWriter, r *http.Request, p string) {
	target := url.URL{Path: p, RawQuery: r.URL.RawQuery}
	http.Redirect(w, r, target.String(), http.StatusMovedPermanently)
}
