package fileserver

import "mime"

// webTypes pins the Content-Type of common web formats. The system MIME
// database, notably the Windows registry, is often wrong for them (".css"
// as "application/x-css", ".js" as "text/plain", ...), which breaks pages in
// browsers since every response is sent with "X-Content-Type-Options: nosniff".
var webTypes = map[string]string{
	".html":        "text/html; charset=utf-8",
	".htm":         "text/html; charset=utf-8",
	".css":         "text/css; charset=utf-8",
	".js":          "text/javascript; charset=utf-8",
	".mjs":         "text/javascript; charset=utf-8",
	".cjs":         "text/javascript; charset=utf-8",
	".json":        "application/json",
	".map":         "application/json",
	".webmanifest": "application/manifest+json",
	".xml":         "text/xml; charset=utf-8",
	".txt":         "text/plain; charset=utf-8",
	".md":          "text/plain; charset=utf-8",
	".csv":         "text/csv; charset=utf-8",
	".svg":         "image/svg+xml",
	".png":         "image/png",
	".jpg":         "image/jpeg",
	".jpeg":        "image/jpeg",
	".gif":         "image/gif",
	".webp":        "image/webp",
	".avif":        "image/avif",
	".ico":         "image/x-icon",
	".bmp":         "image/bmp",
	".woff":        "font/woff",
	".woff2":       "font/woff2",
	".ttf":         "font/ttf",
	".otf":         "font/otf",
	".wasm":        "application/wasm",
	".pdf":         "application/pdf",
	".mp4":         "video/mp4",
	".webm":        "video/webm",
	".ogv":         "video/ogg",
	".mp3":         "audio/mpeg",
	".ogg":         "audio/ogg",
	".oga":         "audio/ogg",
	".wav":         "audio/wav",
	".m4a":         "audio/mp4",
	".flac":        "audio/flac",
	".zip":         "application/zip",
}

func init() {
	for ext, typ := range webTypes {
		// AddExtensionType only fails for malformed input, which the table
		// above does not contain. It overrides the system database.
		_ = mime.AddExtensionType(ext, typ)
	}
}
