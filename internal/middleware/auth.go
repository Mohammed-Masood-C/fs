package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
)

// BasicAuth requires HTTP basic authentication with the given credentials.
func BasicAuth(user, pass, realm string) Middleware {
	// Comparing fixed-size hashes in constant time leaks neither the
	// credentials nor their length through response timing.
	wantUser := sha256.Sum256([]byte(user))
	wantPass := sha256.Sum256([]byte(pass))
	challenge := `Basic realm="` + realm + `", charset="UTF-8"`

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if u, p, ok := r.BasicAuth(); ok {
				gotUser := sha256.Sum256([]byte(u))
				gotPass := sha256.Sum256([]byte(p))
				userOK := subtle.ConstantTimeCompare(gotUser[:], wantUser[:])
				passOK := subtle.ConstantTimeCompare(gotPass[:], wantPass[:])
				if userOK&passOK == 1 {
					next.ServeHTTP(w, r)
					return
				}
			}
			w.Header().Set("WWW-Authenticate", challenge)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		})
	}
}
