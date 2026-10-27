package fileserver

import (
	"io/fs"
	"net/http"
	"path/filepath"
)

// dirSize is the JSON answer to a folder size request (?size). All numbers
// include everything below the folder, not just its direct contents.
type dirSize struct {
	Bytes   int64 `json:"bytes"`
	Files   int   `json:"files"`
	Folders int   `json:"folders"`
}

// serveDirSize reports the total size of all files in the folder name and
// its sub-folders, and how many files and sub-folders there are. The explorer requests it for each folder after the page
// has loaded, so big folders never delay the page itself.
//
// It counts what a ZIP download of the folder would contain: links to files
// are followed (inside the root only), links to folders are not.
func (h *Handler) serveDirSize(w http.ResponseWriter, r *http.Request, name string) {
	fsys := h.root.FS()
	dir := filepath.ToSlash(name)

	var size dirSize
	err := fs.WalkDir(fsys, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == dir {
				return err
			}
			return nil // skip unreadable entries
		}
		if err := r.Context().Err(); err != nil {
			return err // the visitor left the page
		}
		if d.IsDir() {
			if p != dir {
				size.Folders++
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if d.Type()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			if info, err = fs.Stat(fsys, p); err != nil {
				return nil
			}
		}
		if info.Mode().IsRegular() {
			size.Bytes += info.Size()
			size.Files++
		}
		return nil
	})
	if err != nil {
		if r.Context().Err() == nil {
			h.serveFSError(w, r, err)
		}
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	h.writeJSON(w, r, size)
}
