// Package server runs the HTTPS server with production timeouts and graceful
// shutdown. Plain-HTTP requests on the same port are redirected to HTTPS.
package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// readHeaderTimeout bounds how long a client may take to send request
	// headers, which protects against slowloris-style attacks.
	readHeaderTimeout = 10 * time.Second
	// readTimeout bounds reading a whole request. There is deliberately no
	// write timeout, so large downloads over slow links are not cut off.
	readTimeout = 30 * time.Second
	// idleTimeout closes keep-alive connections that stay unused.
	idleTimeout = 2 * time.Minute
	// shutdownTimeout is the grace period for in-flight requests on shutdown.
	shutdownTimeout = 10 * time.Second
	maxHeaderBytes  = 1 << 20
)

// Config configures a Server.
type Config struct {
	Addr        string          // host:port to listen on
	Certificate tls.Certificate // certificate presented to clients
}

// Server is an HTTPS server that is already bound to its address.
type Server struct {
	https    *http.Server
	http     *http.Server // redirects plain-HTTP requests to HTTPS
	listener net.Listener
	mux      *connMux
	logger   *slog.Logger
}

// New creates a server for handler and binds it to cfg.Addr, so that bind
// errors are reported before anything is served.
func New(cfg Config, handler http.Handler, logger *slog.Logger) (*Server, error) {
	ln, err := listen(cfg.Addr)
	if err != nil {
		return nil, err
	}
	errorLog := log.New(errorLogWriter{logger}, "", 0)
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	return &Server{
		https: &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			IdleTimeout:       idleTimeout,
			MaxHeaderBytes:    maxHeaderBytes,
			ErrorLog:          errorLog,
			TLSConfig: &tls.Config{
				MinVersion:   tls.VersionTLS12,
				Certificates: []tls.Certificate{cfg.Certificate},
			},
		},
		http: &http.Server{
			Handler:           redirectToHTTPS(port),
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			IdleTimeout:       10 * time.Second,
			MaxHeaderBytes:    maxHeaderBytes,
			ErrorLog:          errorLog,
		},
		listener: ln,
		mux:      newConnMux(ln),
		logger:   logger,
	}, nil
}

// Addr returns the address the server is listening on.
func (s *Server) Addr() net.Addr { return s.listener.Addr() }

// Run serves requests until ctx is cancelled. It then stops accepting new
// connections and waits up to the shutdown timeout for in-flight requests
// before closing the remaining connections.
func (s *Server) Run(ctx context.Context) error {
	errc := make(chan error, 3)
	go func() { errc <- s.mux.serve() }()
	// The certificate is already in TLSConfig, hence no file names.
	go func() { errc <- s.https.ServeTLS(s.mux.tls, "", "") }()
	go func() { errc <- s.http.Serve(s.mux.plain) }()

	select {
	case err := <-errc:
		s.listener.Close()
		s.https.Close()
		s.http.Close()
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	s.logger.Info("shutting down", "grace_period", shutdownTimeout)
	s.listener.Close() // stop accepting new connections
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := errors.Join(s.https.Shutdown(shutdownCtx), s.http.Shutdown(shutdownCtx)); err != nil {
		s.https.Close()
		s.http.Close()
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("shutdown: requests still running after %s were aborted", shutdownTimeout)
		}
		return fmt.Errorf("shutdown: %w", err)
	}
	s.logger.Info("server stopped")
	return nil
}

// redirectToHTTPS sends plain-HTTP requests to the same URL over HTTPS on
// the same port.
func redirectToHTTPS(port string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		} else {
			host = strings.Trim(host, "[]") // bare IPv6 literal
		}
		if host == "" {
			http.Error(w, "Bad Request: missing Host header", http.StatusBadRequest)
			return
		}
		if port != "443" {
			host = net.JoinHostPort(host, port)
		} else if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		target := url.URL{Scheme: "https", Host: host, Path: r.URL.Path, RawQuery: r.URL.RawQuery}
		// Temporary, so browsers do not remember it if something else later
		// serves plain HTTP on this port.
		http.Redirect(w, r, target.String(), http.StatusTemporaryRedirect)
	})
}

// errorLogWriter routes net/http's internal error log to slog. Failed TLS
// handshakes are dropped: with a self-signed certificate, every browser that
// has not yet accepted it aborts one, which is expected and not actionable.
type errorLogWriter struct{ logger *slog.Logger }

func (w errorLogWriter) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	if !strings.Contains(msg, "TLS handshake error") {
		w.logger.Warn(msg)
	}
	return len(p), nil
}
