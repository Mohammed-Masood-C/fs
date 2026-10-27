package middleware

import (
	"io"
	"net/http"
)

// responseRecorder wraps an http.ResponseWriter to record the status code and
// the number of body bytes written. It keeps the io.ReaderFrom fast path of
// the underlying writer, which lets static files be sent with sendfile.
type responseRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int64
	wroteHeader bool
}

// record returns w as a responseRecorder, wrapping it only once even if
// several middleware ask for it.
func record(w http.ResponseWriter) *responseRecorder {
	if rec, ok := w.(*responseRecorder); ok {
		return rec
	}
	return &responseRecorder{ResponseWriter: w}
}

func (r *responseRecorder) markWritten(status int) {
	if !r.wroteHeader {
		r.status = status
		r.wroteHeader = true
	}
}

func (r *responseRecorder) WriteHeader(status int) {
	// 1xx responses are informational; the final status comes later.
	if status >= 200 {
		r.markWritten(status)
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(p []byte) (int, error) {
	r.markWritten(http.StatusOK)
	n, err := r.ResponseWriter.Write(p)
	r.bytes += int64(n)
	return n, err
}

// ReadFrom implements io.ReaderFrom so that io.Copy (used by
// http.ServeContent) keeps the zero-copy path of the underlying writer.
func (r *responseRecorder) ReadFrom(src io.Reader) (int64, error) {
	r.markWritten(http.StatusOK)
	var (
		n   int64
		err error
	)
	if rf, ok := r.ResponseWriter.(io.ReaderFrom); ok {
		n, err = rf.ReadFrom(src)
	} else {
		n, err = io.Copy(writerOnly{r.ResponseWriter}, src)
	}
	r.bytes += n
	return n, err
}

// Flush implements http.Flusher.
func (r *responseRecorder) Flush() {
	r.markWritten(http.StatusOK)
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap gives http.ResponseController access to the underlying writer.
func (r *responseRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// Status returns the response status code; 200 if nothing was written yet.
func (r *responseRecorder) Status() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

// writerOnly hides every method of an io.Writer except Write, which keeps
// io.Copy from calling back into ReadFrom.
type writerOnly struct{ io.Writer }
