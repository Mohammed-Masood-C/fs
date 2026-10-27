package fileserver

import (
	"archive/zip"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strings"
)

// storedExts are formats that are already compressed. Deflating them again
// costs CPU and saves next to nothing, so they are stored as-is.
var storedExts = map[string]bool{
	".zip": true, ".gz": true, ".tgz": true, ".bz2": true, ".xz": true, ".zst": true,
	".7z": true, ".rar": true, ".jar": true, ".apk": true,
	".docx": true, ".xlsx": true, ".pptx": true, ".odt": true, ".ods": true, ".odp": true,
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".avif": true, ".heic": true,
	".mp4": true, ".m4v": true, ".mkv": true, ".webm": true, ".mov": true, ".avi": true,
	".mp3": true, ".m4a": true, ".aac": true, ".ogg": true, ".opus": true, ".flac": true,
	".woff": true, ".woff2": true,
}

// serveZip streams the folder name, including all sub-folders, as a ZIP
// archive. The archive is written while the folder is walked, so memory use
// stays flat regardless of the folder's size.
func (h *Handler) serveZip(w http.ResponseWriter, r *http.Request, name string) {
	dir := filepath.ToSlash(name)
	label := h.rootLabel
	if dir != "." {
		label = path.Base(dir)
	}

	hdr := w.Header()
	hdr.Set("Content-Type", "application/zip")
	hdr.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": label + ".zip"}))
	hdr.Set("Cache-Control", "no-store")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}

	fsys := h.root.FS()
	zw := zip.NewWriter(w)
	err := fs.WalkDir(fsys, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == dir {
				return err
			}
			h.logger.Warn("zip: skipping unreadable entry", "path", p, "err", err)
			return nil
		}
		if err := r.Context().Err(); err != nil {
			return err // client went away
		}
		if p == dir {
			return nil
		}
		rel := p
		if dir != "." {
			rel = strings.TrimPrefix(p, dir+"/")
		}
		return h.addToZip(zw, fsys, p, rel, d)
	})
	if err == nil {
		err = zw.Close()
	}
	if err != nil {
		if r.Context().Err() == nil {
			h.logger.Error("zip download failed", "path", r.URL.Path, "err", err)
		}
		// The response has already started. Abort the connection so the client
		// does not mistake a truncated archive for a complete one.
		panic(http.ErrAbortHandler)
	}
}

// addToZip adds the entry p of fsys to the archive under the name rel.
func (h *Handler) addToZip(zw *zip.Writer, fsys fs.FS, p, rel string, d fs.DirEntry) error {
	info, err := d.Info()
	if err != nil {
		return nil // removed while walking
	}
	if d.Type()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
		// Links are followed (inside the root only) to files but not to folders,
		// which could otherwise make the archive loop forever.
		if info, err = fs.Stat(fsys, p); err != nil || !info.Mode().IsRegular() {
			return nil
		}
	}

	switch {
	case info.IsDir():
		// An explicit entry keeps empty folders in the archive.
		_, err := zw.CreateHeader(&zip.FileHeader{Name: rel + "/", Modified: info.ModTime()})
		return err
	case !info.Mode().IsRegular():
		return nil // devices, sockets and named pipes
	}

	f, err := fsys.Open(p)
	if err != nil {
		h.logger.Warn("zip: skipping unreadable file", "path", p, "err", err)
		return nil
	}
	defer f.Close()

	fh := &zip.FileHeader{Name: rel, Modified: info.ModTime(), Method: zip.Deflate}
	if storedExts[strings.ToLower(path.Ext(rel))] {
		fh.Method = zip.Store
	}
	dst, err := zw.CreateHeader(fh)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, f)
	return err
}
