package fileserver

import (
	"cmp"
	"io/fs"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"time"
)

// Columns the explorer can be sorted by.
const (
	sortName     = "name"
	sortSize     = "size"
	sortModified = "modified"
)

// entry is one item of a folder.
type entry struct {
	Name    string
	IsDir   bool
	IsLink  bool
	Size    int64
	ModTime time.Time
}

// viewParams is the explorer state carried in the query string.
type viewParams struct {
	sort string
	desc bool
	page int
}

func parseView(q url.Values) viewParams {
	v := viewParams{sort: sortName, page: 1}
	switch s := q.Get("sort"); s {
	case sortSize, sortModified:
		v.sort = s
	}
	v.desc = q.Get("order") == "desc"
	if p, err := strconv.Atoi(q.Get("page")); err == nil && p > 1 {
		v.page = p
	}
	return v
}

// query returns the query string that selects v, or "" for the default view.
func (v viewParams) query() string {
	q := url.Values{}
	if v.sort != sortName {
		q.Set("sort", v.sort)
	}
	if v.desc {
		q.Set("order", "desc")
	}
	if v.page > 1 {
		q.Set("page", strconv.Itoa(v.page))
	}
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

// serveListing renders the explorer for the folder name, or its JSON form
// when the query string contains format=json.
func (h *Handler) serveListing(w http.ResponseWriter, r *http.Request, name, urlPath string) {
	entries, err := h.readDir(name)
	if err != nil {
		h.serveFSError(w, r, err)
		return
	}
	query := r.URL.Query()
	view := parseView(query)
	sortEntries(entries, view.sort, view.desc)

	w.Header().Set("Cache-Control", "no-cache")
	if query.Get("format") == "json" {
		h.writeJSON(w, r, newJSONListing(urlPath, entries))
		return
	}
	nonce := newNonce()
	h.writeHTML(w, r, http.StatusOK, h.tmpl.listing, h.newListingView(urlPath, entries, view, nonce), nonce)
}

// readDir lists the folder name, including hidden files. Special files and
// symbolic links that cannot be resolved inside the root are left out, so
// everything listed can also be opened.
func (h *Handler) readDir(name string) ([]entry, error) {
	f, err := h.root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	dirents, err := f.ReadDir(-1)
	if err != nil {
		return nil, err
	}

	entries := make([]entry, 0, len(dirents))
	for _, d := range dirents {
		if !validURLPath("/" + d.Name()) {
			continue
		}
		info, err := d.Info()
		if err != nil {
			continue // removed while listing
		}
		// Symbolic links, and on Windows junctions and other reparse points,
		// are resolved through the root; those leading outside of it fail here.
		isLink := info.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0
		if isLink {
			if info, err = h.root.Stat(filepath.Join(name, d.Name())); err != nil {
				continue
			}
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			continue
		}
		entries = append(entries, entry{
			Name:    d.Name(),
			IsDir:   info.IsDir(),
			IsLink:  isLink,
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}
	return entries, nil
}

// sortEntries sorts folders before files, then by the given column.
func sortEntries(entries []entry, by string, desc bool) {
	slices.SortFunc(entries, func(a, b entry) int {
		if a.IsDir != b.IsDir {
			if a.IsDir {
				return -1
			}
			return 1
		}
		var c int
		switch by {
		case sortSize:
			if !a.IsDir { // folder sizes are meaningless; they sort by name
				c = cmp.Compare(a.Size, b.Size)
			}
		case sortModified:
			c = a.ModTime.Compare(b.ModTime)
		}
		if c == 0 {
			c = compareNames(a.Name, b.Name)
		}
		if desc {
			c = -c
		}
		return c
	})
}
