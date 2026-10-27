package fileserver

import (
	"errors"
	"io/fs"
	"net/http"
	"path"
	"runtime"
	"strings"
	"syscall"
)

// windowsInvalidChars may not appear in Windows file names. ':' would also
// give access to NTFS alternate data streams ("file.txt::$DATA").
const windowsInvalidChars = `<>:"|?*`

// validURLPath reports whether the decoded URL path p can be mapped onto the
// file system at all. Backslashes are rejected everywhere: they are path
// separators on Windows, and browsers treat "/\" like "//".
func validURLPath(p string) bool {
	if strings.ContainsAny(p, "\\\x00") {
		return false
	}
	if runtime.GOOS == "windows" {
		if strings.ContainsAny(p, windowsInvalidChars) {
			return false
		}
		for i := 0; i < len(p); i++ {
			if p[i] < 0x20 {
				return false
			}
		}
	}
	return true
}

// cleanURLPath returns the canonical form of an absolute URL path: no "." or
// ".." elements and no repeated slashes, with any trailing slash kept.
func cleanURLPath(p string) string {
	if p == "" || p[0] != '/' {
		p = "/" + p
	}
	clean := path.Clean(p)
	if clean != "/" && strings.HasSuffix(p, "/") {
		clean += "/"
	}
	return clean
}

// statusForError maps a file system error to an HTTP status code.
func statusForError(err error) int {
	switch {
	case errors.Is(err, fs.ErrNotExist),
		errors.Is(err, syscall.ENOTDIR),
		errors.Is(err, syscall.ENAMETOOLONG),
		errors.Is(err, syscall.ELOOP):
		return http.StatusNotFound
	case errors.Is(err, fs.ErrPermission):
		return http.StatusForbidden
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return http.StatusInternalServerError
	}
	// Errors that do not come from the operating system are raised by os.Root
	// when a path would escape the folder, for example through a symbolic link.
	return http.StatusNotFound
}
