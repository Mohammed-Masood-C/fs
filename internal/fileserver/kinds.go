package fileserver

import (
	"path"
	"strconv"
	"strings"
)

// kindByExt maps lowercase file extensions to the icon the explorer shows.
var kindByExt = map[string]string{}

func init() {
	groups := map[string][]string{
		"image": {".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".bmp", ".ico", ".svg",
			".tif", ".tiff", ".heic", ".psd"},
		"video": {".mp4", ".m4v", ".mkv", ".webm", ".mov", ".avi", ".wmv", ".flv", ".mpg",
			".mpeg", ".ogv", ".3gp"},
		"audio": {".mp3", ".wav", ".flac", ".aac", ".ogg", ".oga", ".opus", ".m4a", ".wma",
			".mid", ".midi"},
		"archive": {".zip", ".rar", ".7z", ".tar", ".gz", ".tgz", ".bz2", ".xz", ".zst",
			".iso", ".cab", ".jar", ".war"},
		"pdf": {".pdf"},
		"code": {".html", ".htm", ".css", ".scss", ".sass", ".less", ".js", ".mjs", ".cjs",
			".jsx", ".ts", ".tsx", ".vue", ".svelte", ".json", ".xml", ".yaml", ".yml",
			".toml", ".go", ".py", ".rb", ".php", ".java", ".kt", ".swift", ".c", ".h",
			".cpp", ".hpp", ".cc", ".cs", ".rs", ".sh", ".bash", ".ps1", ".bat", ".cmd",
			".sql", ".lua", ".pl", ".r", ".dart", ".scala", ".wasm"},
		"text": {".txt", ".md", ".markdown", ".rst", ".log", ".ini", ".cfg", ".conf",
			".env", ".csv", ".tsv", ".rtf"},
		"doc": {".doc", ".docx", ".odt", ".pages", ".xls", ".xlsx", ".ods", ".numbers",
			".ppt", ".pptx", ".odp", ".key", ".epub"},
	}
	for kind, exts := range groups {
		for _, ext := range exts {
			kindByExt[ext] = kind
		}
	}
}

// fileKind returns the icon kind of a file name.
func fileKind(name string) string {
	if kind, ok := kindByExt[strings.ToLower(path.Ext(name))]; ok {
		return kind
	}
	return "file"
}

// humanSize formats a byte count with binary units, as file explorers do:
// 1536 becomes "1.5 KB".
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit && exp < 5; m /= unit {
		div *= unit
		exp++
	}
	return strconv.FormatFloat(float64(n)/float64(div), 'f', 1, 64) + " " + string("KMGTPE"[exp]) + "B"
}
