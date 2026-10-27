package fileserver

import (
	"fmt"
	"io/fs"
	"net/http"
)

// serveFile serves the regular file name. http.ServeContent takes care of
// HEAD, Range and If-Range requests, conditional requests (If-None-Match,
// If-Modified-Since) and the Content-Type, which comes from the extension or,
// failing that, from sniffing the content.
func (h *Handler) serveFile(w http.ResponseWriter, r *http.Request, name string) {
	f, err := h.root.Open(name)
	if err != nil {
		h.serveFSError(w, r, err)
		return
	}
	defer f.Close()

	// Stat the open handle: the file may have been replaced since its path
	// was resolved.
	info, err := f.Stat()
	if err != nil {
		h.serveFSError(w, r, err)
		return
	}
	if !info.Mode().IsRegular() {
		h.serveError(w, r, http.StatusNotFound)
		return
	}

	hdr := w.Header()
	hdr.Set("Cache-Control", cacheControl)
	hdr.Set("ETag", etag(info))
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

// etag derives a strong validator from the modification time and size of a
// file, like nginx does. Being strong, it also works with If-Range.
func etag(info fs.FileInfo) string {
	return fmt.Sprintf(`"%x-%x"`, info.ModTime().UnixNano(), info.Size())
}
