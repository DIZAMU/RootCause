// Package server is the local dashboard (spec section 11): a JSON API over
// the same session engine, selector and store the CLI uses, and the built
// React app from web/. It binds to 127.0.0.1 only (spec section 13.1).
//
// Hardening against other websites in the learner's browser: requests whose
// Host header is not 127.0.0.1:PORT or localhost:PORT are refused, which
// blocks DNS rebinding; every /api request must carry a per-process token
// that only the served index.html contains, which blocks cross-site
// requests; and the server never sends CORS headers.
package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DIZAMU/RootCause/internal/i18n"
	"github.com/DIZAMU/RootCause/internal/session"
	"github.com/DIZAMU/RootCause/internal/store"
)

// BindAddress is the only address the dashboard listens on.
const BindAddress = "127.0.0.1"

// DefaultPort is the dashboard's port (spec section 8.1).
const DefaultPort = 7300

// ContentSecurityPolicy is sent with every response.
const ContentSecurityPolicy = "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; " +
	"frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

// TokenHeader carries the per-process token on every /api request.
const TokenHeader = "X-RootCause-Token"

// tokenPlaceholder is replaced by the token in the served index.html.
const tokenPlaceholder = "__RC_TOKEN__"

// Config is what the server needs. Every field except Assets, Random and
// CanRebuild is required.
type Config struct {
	Store        *store.Store
	Engine       *session.Engine
	Bundle       *i18n.Bundle
	LearnerID    int
	ExercisesDir string
	// Catalog loads the exercise catalog, the same way the CLI does.
	Catalog func() ([]store.ExerciseRow, error)
	// BuildStats rebuilds the marts (play.BuildStats in production).
	BuildStats func(ctx context.Context) error
	// Describe renders an error for the learner in lang, as the CLI does.
	Describe func(lang string, err error) string
	// Assets is the built dashboard (web/dist). nil means it was not built,
	// and / serves a page that says how to build it.
	Assets fs.FS
	// Random returns seeds for new sessions. nil means math/rand.
	Random func() int64
	// CanRebuild is true where BuildStats really builds the marts, so the
	// dashboard may offer "rebuild now". It is false in the cluster, where a
	// CronJob builds them.
	CanRebuild bool
}

// Server serves the dashboard.
type Server struct {
	cfg   Config
	token string
	stats *refresher

	// actions serializes the session actions (start, hint, check, reset,
	// give up) and the data reset, so a double click cannot run two at once.
	actions sync.Mutex

	// starting is the session start in progress, for the overview; nil when
	// none runs. Guarded by startMu.
	startMu  sync.Mutex
	starting *startingJSON
}

// New builds a server with a fresh random token. Background statistics
// rebuilds run under ctx; cancel it to stop them.
func New(ctx context.Context, cfg Config) (*Server, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("token: %w", err)
	}
	s := &Server{cfg: cfg, token: hex.EncodeToString(b)}
	s.stats = newRefresher(ctx, cfg.BuildStats, func(ctx context.Context) (bool, error) {
		return cfg.Store.MartsFresh(ctx, cfg.LearnerID)
	})
	return s, nil
}

// Token returns the server's API token.
func (s *Server) Token() string { return s.token }

// Listen opens the dashboard's listener on 127.0.0.1:port. Port 0 picks a
// free port.
func Listen(port int) (net.Listener, error) {
	return net.Listen("tcp", net.JoinHostPort(BindAddress, strconv.Itoa(port)))
}

// ListenAll opens the dashboard's listener on every address of port. Only the
// cluster deployment uses it: the pod must accept connections from its
// Service, and k3d publishes the port on the host's 127.0.0.1 only. Host and
// token checks apply as on 127.0.0.1.
func ListenAll(port int) (net.Listener, error) {
	return net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(port)))
}

// Serve serves on ln until ctx ends, then shuts down: it cancels requests
// still running (a dbt build among them removes its container), waits for
// them, and waits for background statistics rebuilds to stop.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	port := ln.Addr().(*net.TCPAddr).Port
	reqCtx, cancelReqs := context.WithCancel(ctx)
	defer cancelReqs()
	srv := &http.Server{
		Handler:           s.Handler(port),
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return reqCtx },
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	select {
	case err := <-errc:
		s.stats.wait()
		return err
	case <-ctx.Done():
	}
	cancelReqs()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := srv.Shutdown(shutdownCtx)
	s.stats.wait()
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return err
}

// Handler returns the dashboard's handler for a server on port.
func (s *Server) Handler(port int) http.Handler {
	mux := http.NewServeMux()
	s.routes(mux)
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /index.html", s.handleIndex)
	if s.cfg.Assets != nil {
		mux.Handle("GET /assets/", http.FileServerFS(s.cfg.Assets))
	}
	return s.guard(port, mux)
}

// guard applies the hardening described in the package comment, and the
// security headers every response gets.
func (s *Server) guard(port int, next http.Handler) http.Handler {
	p := strconv.Itoa(port)
	allowed := map[string]bool{BindAddress + ":" + p: true, "localhost:" + p: true}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		// Inline styles only: the web terminal (xterm.js) writes its layout
		// and colors into <style> elements it creates. Scripts stay 'self'.
		h.Set("Content-Security-Policy", ContentSecurityPolicy)
		if !allowed[strings.ToLower(r.Host)] {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
			got := r.Header.Get(TokenHeader)
			if r.URL.Path == TerminalPath {
				if !sameOrigin(r) {
					http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
					return
				}
				got = protocolToken(r)
			}
			if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// handleIndex serves the app's index.html with the token filled in, or, if
// the dashboard was not built, a translated page saying how to build it.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if s.cfg.Assets == nil {
		lang := s.lang(r.Context())
		_ = notBuiltPage.Execute(w, map[string]string{
			"Lang":  lang,
			"Title": s.cfg.Bundle.T(lang, "web.app_name", nil),
			"Text":  s.cfg.Bundle.T(lang, "web.not_built", nil),
		})
		return
	}
	page, err := fs.ReadFile(s.cfg.Assets, "index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(bytes.ReplaceAll(page, []byte(tokenPlaceholder), []byte(s.token)))
}

// notBuiltPage is served at / when the binary has no dashboard.
var notBuiltPage = template.Must(template.New("not-built").Parse(`<!doctype html>
<html lang="{{.Lang}}"><head><meta charset="utf-8"><title>{{.Title}}</title></head>
<body><h1>{{.Title}}</h1><p>{{.Text}}</p></body></html>
`))

// lang is the learner's current language. The CLI may change it at any time,
// so it is read on every request. It falls back to English.
func (s *Server) lang(ctx context.Context) string {
	l, err := s.cfg.Store.LearnerByID(ctx, s.cfg.LearnerID)
	if err != nil || l.Lang == "" {
		return "en"
	}
	return l.Lang
}
