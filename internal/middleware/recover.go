package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recover turns a panic in a downstream handler into a 500 response and logs
// it with a stack trace, instead of dropping the connection silently.
func Recover(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := record(w)
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(v) // deliberate abort; net/http handles it quietly
				}
				logger.Error("panic while serving request",
					"method", r.Method,
					"uri", r.URL.RequestURI(),
					"panic", v,
					"stack", string(debug.Stack()),
				)
				if rec.wroteHeader {
					// The response is already on its way; abort the connection so the
					// client sees a truncated response rather than a corrupt one.
					panic(http.ErrAbortHandler)
				}
				h := rec.Header()
				for _, k := range []string{"Content-Length", "Content-Encoding", "Content-Range", "Content-Disposition", "ETag", "Last-Modified"} {
					h.Del(k)
				}
				http.Error(rec, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}()
			next.ServeHTTP(rec, r)
		})
	}
}
