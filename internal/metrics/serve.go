// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// contentType is the plain-text exposition format. Prometheus accepts it without
// negotiation, which is why there is no protobuf path here.
const contentType = "text/plain; version=0.0.4; charset=utf-8"

// Handler serves the registry at /metrics.
func Handler(r *Registry) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet && req.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", contentType)
		if req.Method == http.MethodHead {
			return
		}
		fmt.Fprint(w, r.Render())
	})
	// Anything else gets named instead of a silent 404, since the path is the one thing a
	// scrape configuration gets wrong.
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		http.Error(w, "nothing here, metrics are at /metrics", http.StatusNotFound)
	})
	return mux
}

// Server is the listener, held so the agent can stop it.
type Server struct {
	http *http.Server
	ln   net.Listener
}

// Listen starts serving. The address is bound before returning, so a port already in
// use is an error at startup, not a service that looks healthy and answers nothing.
func Listen(address string, r *Registry) (*Server, error) {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("serving metrics on %s: %w", address, err)
	}
	srv := &Server{
		http: &http.Server{
			Handler: Handler(r),
			// A scrape is a small local request, so these are short. They exist to
			// stop a stuck client holding a connection in a root process.
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
		ln: ln,
	}
	go func() {
		// Serve always returns non-nil. A closed listener is the ordinary shutdown path, so
		// there is nothing to report.
		if err := srv.http.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return
		}
	}()
	return srv, nil
}

// Address is what the listener actually bound, which differs from the configured
// value when port 0 was asked for.
func (s *Server) Address() string {
	if s == nil || s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

func (s *Server) Close() error {
	if s == nil || s.http == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return s.http.Shutdown(ctx)
}

// WriteTextfile writes the same metrics for a textfile collector.
//
// Written and renamed, because a collector reading a half-written file reports a parse
// error for the whole scrape. Where only the file is used the absolute-timestamp rule
// is what makes a stopped agent detectable, since the collector keeps serving the last
// file it found.
func WriteTextfile(path string, r *Registry) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temp := path + ".tmp"
	// World-readable, because a textfile collector usually runs as a different user from
	// the agent. Nothing in the catalogue is secret.
	if err := os.WriteFile(temp, []byte(r.Render()), 0o644); err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		os.Remove(temp)
		return err
	}
	return nil
}
