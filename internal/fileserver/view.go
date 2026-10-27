package fileserver

import (
	"html/template"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// pageSize is the maximum number of entries on one explorer page. It bounds
// the size of the page for folders with many thousands of files.
const pageSize = 1000

// listingView is the data of the explorer template. Every Href is an
// escaped, absolute URL path.
type listingView struct {
	Title   string
	Crumbs  []crumb
	Columns []column
	Parent  string // empty in the root folder
	Zip     string // downloads the whole folder as a ZIP archive
	Rows    []row
	Summary string
	Page    int
	Pages   int
	Prev    string
	Next    string
	Nonce   string
	CSS     template.CSS
	JS      template.JS
}

type crumb struct {
	Name    string
	Href    string
	Current bool
}

type column struct {
	Key    string
	Label  string
	Href   string
	Sorted string // "asc" or "desc" for the active column, "" otherwise
}

type row struct {
	Name     string
	Href     string
	Download string // the file itself, or a ZIP archive for folders
	Kind     string
	IsDir    bool
	IsLink   bool
	Size     string
	Bytes    int64
	Modified string
	ISO      string
}

var columnDefs = [...]struct{ key, label string }{
	{sortName, "Name"},
	{sortSize, "Size"},
	{sortModified, "Modified"},
}

func (h *Handler) newListingView(urlPath string, entries []entry, v viewParams, nonce string) listingView {
	base := escapePath(urlPath)
	// Links to folders keep the current sort order while navigating.
	dirQuery := viewParams{sort: v.sort, desc: v.desc, page: 1}.query()

	pages := max(1, (len(entries)+pageSize-1)/pageSize)
	page := min(v.page, pages)
	start := (page - 1) * pageSize
	end := min(start+pageSize, len(entries))

	view := listingView{
		Title:   urlPath,
		Crumbs:  breadcrumbs(urlPath, h.rootLabel, dirQuery),
		Columns: sortColumns(base, v),
		Zip:     base + "?download",
		Rows:    make([]row, 0, end-start),
		Summary: summarize(entries),
		Page:    page,
		Pages:   pages,
		Nonce:   nonce,
		CSS:     h.tmpl.css,
		JS:      h.tmpl.js,
	}
	for _, e := range entries[start:end] {
		view.Rows = append(view.Rows, newRow(base, e, dirQuery))
	}
	if urlPath != "/" {
		parent := path.Dir(strings.TrimSuffix(base, "/"))
		if parent != "/" {
			parent += "/"
		}
		view.Parent = parent + dirQuery
	}
	if page > 1 {
		view.Prev = base + viewParams{sort: v.sort, desc: v.desc, page: page - 1}.query()
	}
	if page < pages {
		view.Next = base + viewParams{sort: v.sort, desc: v.desc, page: page + 1}.query()
	}
	return view
}

func newRow(base string, e entry, dirQuery string) row {
	r := row{
		Name:     e.Name,
		Href:     base + url.PathEscape(e.Name),
		IsDir:    e.IsDir,
		IsLink:   e.IsLink,
		Modified: e.ModTime.Format("2006-01-02 15:04"),
		ISO:      e.ModTime.UTC().Format(time.RFC3339),
	}
	if e.IsDir {
		r.Kind = "folder"
		r.Download = r.Href + "/?download"
		r.Href += "/" + dirQuery
	} else {
		r.Kind = fileKind(e.Name)
		r.Download = r.Href
		r.Size = humanSize(e.Size)
		r.Bytes = e.Size
	}
	return r
}

// breadcrumbs returns the path from the root folder to urlPath.
func breadcrumbs(urlPath, label, query string) []crumb {
	crumbs := []crumb{{Name: label, Href: "/" + query}}
	href := "/"
	for _, seg := range strings.Split(strings.Trim(urlPath, "/"), "/") {
		if seg == "" {
			continue
		}
		href += url.PathEscape(seg) + "/"
		crumbs = append(crumbs, crumb{Name: seg, Href: href + query})
	}
	crumbs[len(crumbs)-1].Current = true
	return crumbs
}

// sortColumns returns the column headers. Clicking the active column flips
// the order; clicking another one sorts by it in ascending order.
func sortColumns(base string, v viewParams) []column {
	cols := make([]column, 0, len(columnDefs))
	for _, d := range columnDefs {
		c := column{Key: d.key, Label: d.label}
		next := viewParams{sort: d.key, page: 1}
		if v.sort == d.key {
			c.Sorted = "asc"
			if v.desc {
				c.Sorted = "desc"
			}
			next.desc = !v.desc
		}
		c.Href = base + next.query()
		cols = append(cols, c)
	}
	return cols
}

// summarize describes the content of a folder, e.g. "2 folders, 7 files, 3.1 MB".
func summarize(entries []entry) string {
	var dirs, files int
	var size int64
	for _, e := range entries {
		if e.IsDir {
			dirs++
		} else {
			files++
			size += e.Size
		}
	}
	s := plural(dirs, "folder") + ", " + plural(files, "file")
	if files > 0 {
		s += ", " + humanSize(size)
	}
	return s
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}

// escapePath escapes a URL path for use in a link.
func escapePath(p string) string {
	return (&url.URL{Path: p}).EscapedPath()
}

// rootLabel names the root folder in the breadcrumbs. Only the base name is
// shown, so the location of the folder on the server is not disclosed.
func rootLabel(dir string) string {
	name := filepath.Base(dir)
	if name == "." || name == string(filepath.Separator) {
		return "Home"
	}
	return name
}

// jsonListing is the machine-readable form of a folder (?format=json).
type jsonListing struct {
	Path    string      `json:"path"`
	Entries []jsonEntry `json:"entries"`
}

type jsonEntry struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Type     string    `json:"type"` // "dir" or "file"
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	Symlink  bool      `json:"symlink,omitempty"`
}

func newJSONListing(urlPath string, entries []entry) jsonListing {
	base := escapePath(urlPath)
	out := jsonListing{Path: urlPath, Entries: make([]jsonEntry, 0, len(entries))}
	for _, e := range entries {
		je := jsonEntry{
			Name:     e.Name,
			Path:     base + url.PathEscape(e.Name),
			Type:     "file",
			Size:     e.Size,
			Modified: e.ModTime.UTC(),
			Symlink:  e.IsLink,
		}
		if e.IsDir {
			je.Type = "dir"
			je.Path += "/"
			je.Size = 0
		}
		out.Entries = append(out.Entries, je)
	}
	return out
}
