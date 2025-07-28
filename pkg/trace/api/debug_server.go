// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build !serverless

package api

import (
	"context"
	"crypto/tls"
	"expvar"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"runtime"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/api/apiutil"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

const (
	defaultTimeout          = 5 * time.Second
	defaultShutdownDeadline = 5 * time.Second
)

// DebugServer serves /debug/* endpoints
type DebugServer struct {
	conf      *config.AgentConfig
	server    *http.Server
	mux       *http.ServeMux
	tlsConfig *tls.Config
}

// NewDebugServer returns a debug server
func NewDebugServer(conf *config.AgentConfig) *DebugServer {
	return &DebugServer{
		conf: conf,
		mux:  http.NewServeMux(),
	}
}

// Start configures and starts the http server
func (ds *DebugServer) Start() {
	if ds.conf.DebugServerPort == 0 {
		log.Debug("Debug server is disabled by config (apm_config.debug.port: 0).")
		return
	}

	// TODO: Improve certificate delivery
	if ds.tlsConfig == nil {
		log.Warnf("Debug server wasn't able to start: uninitialized IPC certificate")
		return
	}

	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(ds.conf.DebugServerPort)))
	if err != nil {
		log.Errorf("Error creating debug server listener: %s", err)
		return
	}

	ds.server = &http.Server{
		ReadTimeout:  defaultTimeout,
		WriteTimeout: defaultTimeout,
		Handler:      ds.setupMux(),
	}

	tlsListener := tls.NewListener(listener, ds.tlsConfig)
	go func() {
		if err := ds.server.Serve(tlsListener); err != nil && err != http.ErrServerClosed {
			log.Errorf("Could not start debug server: %s. Debug server disabled.", err)
		}
	}()
}

// Stop shuts down the debug server
func (ds *DebugServer) Stop() {
	if ds.server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultShutdownDeadline)
	defer cancel()
	if err := ds.server.Shutdown(ctx); err != nil {
		log.Errorf("Error stopping debug server: %s", err)
	}
}

// AddRoute adds a route to the DebugServer
func (ds *DebugServer) AddRoute(route string, handler http.Handler) {
	ds.mux.Handle(route, handler)
}

// SetTLSConfig adds the provided tls.Config to the internal http.Server
func (ds *DebugServer) SetTLSConfig(config *tls.Config) {
	ds.tlsConfig = config
}

func (ds *DebugServer) setupMux() *http.ServeMux {
	ds.mux.HandleFunc("/debug/pprof/", pprof.Index)
	ds.mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	ds.mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	ds.mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	ds.mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	ds.mux.HandleFunc("/debug/blockrate", func(w http.ResponseWriter, r *http.Request) {
		// this endpoint calls runtime.SetBlockProfileRate(v), where v is an optional
		// query string parameter defaulting to 10000 (1 sample per 10μs blocked).
		rate := 10000
		v := r.URL.Query().Get("v")
		if v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				http.Error(w, "v must be an integer", http.StatusBadRequest)
				return
			}
			rate = n
		}
		runtime.SetBlockProfileRate(rate)
		fmt.Fprintf(w, "Block profile rate set to %d. It will automatically be disabled again after calling /debug/pprof/block\n", rate)
	})
	ds.mux.HandleFunc("/debug/pprof/block", func(w http.ResponseWriter, r *http.Request) {
		// serve the block profile and reset the rate to 0.
		pprof.Handler("block").ServeHTTP(w, r)
		runtime.SetBlockProfileRate(0)
	})
	ds.mux.Handle("/debug/vars", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// allow the GUI to call this endpoint so that the status can be reported
		w.Header().Set("Access-Control-Allow-Origin", "http://127.0.0.1:"+ds.conf.GUIPort)
		expvar.Handler().ServeHTTP(w, req)
	}))
	apiutil.SetupCoverageHandler(ds.mux)
	return ds.mux
}
