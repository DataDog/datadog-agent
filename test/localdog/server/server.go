// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package server implements the localdog HTTP server: a Datadog intake for the Agent, and the
// subset of the Datadog UI API needed by the localdog web app to explore what was received.
package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/localdog/store"
)

// Version is the localdog version reported by /info.
var Version = "0.1.0"

// Server is the localdog HTTP server.
type Server struct {
	store   *store.Store
	mux     *http.ServeMux
	ui      fs.FS
	started time.Time
}

// Options configures the server.
type Options struct {
	// UIDir optionally serves a built localdog web app (web-ui static-apps/localdog/dist) at the root.
	// When empty, the web app embedded at build time (if any) is served.
	UIDir string
}

// New creates a server around a store.
func New(st *store.Store, opts Options) *Server {
	s := &Server{store: st, mux: http.NewServeMux(), ui: embeddedUI(), started: time.Now()}
	if opts.UIDir != "" {
		s.ui = os.DirFS(opts.UIDir)
	}

	s.mux.HandleFunc("/info", s.handleInfo)
	s.mux.HandleFunc("/api/localdog/info", s.handleInfo)
	s.mux.HandleFunc("/api/localdog/stats", s.handleStats)
	s.mux.HandleFunc("/api/localdog/reset", s.handleReset)
	s.mux.HandleFunc("/api/localdog/metrics", s.handleMetricsList)
	s.mux.HandleFunc("/api/localdog/metric-tags", s.handleMetricTags)

	// Datadog UI API subset
	s.mux.HandleFunc("/api/ui/trace/", s.handleTrace)
	s.mux.HandleFunc("/api/ui/query/timeseries", s.handleTimeseries)
	s.mux.HandleFunc("/api/ui/query/scalar", s.handleScalar)
	s.mux.HandleFunc("/api/v1/logs-analytics/list", s.handleList)
	s.mux.HandleFunc("/api/v1/logs-analytics/list/", s.handleList)
	s.mux.HandleFunc("/api/v1/logs-analytics/aggregate", s.handleAggregate)
	s.mux.HandleFunc("/api/v1/logs-analytics/aggregate/", s.handleAggregate)
	s.mux.HandleFunc("/api/v1/logs-analytics/facet_info", s.handleFacetInfo)
	s.mux.HandleFunc("/api/v1/logs-analytics/facet_range_info", s.handleFacetInfo)
	s.mux.HandleFunc("/api/v1/logs-analytics/fetch_one", s.handleFetchOne)
	s.mux.HandleFunc("/api/ui/event-platform/", s.handleFacetList)
	s.mux.HandleFunc("/api/v1/logs/indexes", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"indexes": []any{}, "locked": false})
	})
	s.mux.HandleFunc("/api/ui/apm/web/metadata", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{})
	})

	// Everything else is the Agent intake.
	s.mux.HandleFunc("/", s.intakeHandler)
	return s
}

// ServeHTTP adds the CORS headers that let a web app on another origin (e.g. the hosted static
// site, or the web-ui dev server) read from localdog on localhost.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	origin := r.Header.Get("Origin")
	if origin != "" {
		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Access-Control-Allow-Credentials", "true")
		h.Set("Vary", "Origin")
	} else {
		h.Set("Access-Control-Allow-Origin", "*")
	}
	if r.Method == http.MethodOptions {
		h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if req := r.Header.Get("Access-Control-Request-Headers"); req != "" {
			h.Set("Access-Control-Allow-Headers", req)
		} else {
			h.Set("Access-Control-Allow-Headers", "*")
		}
		// Chrome Private Network Access: a public https page may call localhost.
		h.Set("Access-Control-Allow-Private-Network", "true")
		h.Set("Access-Control-Max-Age", "600")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"localdog":   true,
		"version":    Version,
		"started_at": s.started.UnixMilli(),
		"stats":      s.store.Stats(),
	})
}

// HasUI reports whether the server serves the web app.
func (s *Server) HasUI() bool { return s.ui != nil }

// serveUI serves the built web app for browser navigation, with an SPA fallback to index.html.
// It reports false when the request is not for the UI (agent intake and API calls).
func (s *Server) serveUI(w http.ResponseWriter, r *http.Request) bool {
	if s.ui == nil || (r.Method != http.MethodGet && r.Method != http.MethodHead) || strings.HasPrefix(r.URL.Path, "/api/") {
		return false
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name != "" && name != "index.html" {
		if fi, err := fs.Stat(s.ui, name); err == nil && !fi.IsDir() {
			http.ServeFileFS(w, r, s.ui, name)
			return true
		}
		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return true
		}
	}
	index, err := fs.ReadFile(s.ui, "index.html")
	if err != nil {
		http.Error(w, "localdog web app not found", http.StatusNotFound)
		return true
	}
	// tell the app it is served by localdog, so it talks to this origin
	page := strings.Replace(string(index), "<head>", "<head><script>globalThis.LOCALDOG_SERVED=true</script>", 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(page))
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
