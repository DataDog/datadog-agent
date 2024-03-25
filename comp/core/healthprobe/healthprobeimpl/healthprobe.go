// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package healthprobeimpl implements the healthprobe component interface
package healthprobeimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"time"

	"go.uber.org/fx"

	healthprobeComponent "github.com/DataDog/datadog-agent/comp/core/healthprobe"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/gorilla/mux"
)

const defaultTimeout = time.Second

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newHealthProbe),
	)
}

type dependencies struct {
	fx.In

	Log     log.Component
	Options healthprobeComponent.Options
}

type healthprobe struct {
	options  healthprobeComponent.Options
	log      log.Component
	server   *http.Server
	listener net.Listener
}

func (h *healthprobe) start() error {
	h.log.Debugf("Health check listening on port %d", h.options.Port)

	go h.server.Serve(h.listener) //nolint:errcheck

	return nil
}

func (h *healthprobe) stop() error {
	timeout, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	h.log.Debug("Stopping Health check")
	return h.server.Shutdown(timeout) //nolint:errcheck
}

func newHealthProbe(lc fx.Lifecycle, deps dependencies) (healthprobeComponent.Component, error) {
	healthPort := deps.Options.Port
	if healthPort <= 0 {
		return nil, nil
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%v", healthPort))
	if err != nil {
		return nil, err
	}

	server := buildServer(deps.Options, deps.Log)

	probe := &healthprobe{
		options:  deps.Options,
		log:      deps.Log,
		server:   server,
		listener: ln,
	}

	// We rely on FX to start and stop the metadata runner
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return probe.start()
		},
		OnStop: func(ctx context.Context) error {
			return probe.stop()
		},
	})

	return probe, nil
}

type liveHandler struct {
	logsGoroutines bool
	log            log.Component
}

func (lh liveHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	healthHandler(lh.logsGoroutines, lh.log, health.GetLiveNonBlocking, w, r)
}

type readyHandler struct {
	logsGoroutines bool
	log            log.Component
}

func (rh readyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	healthHandler(rh.logsGoroutines, rh.log, health.GetReadyNonBlocking, w, r)
}

func buildServer(options healthprobeComponent.Options, log log.Component) *http.Server {
	r := mux.NewRouter()

	liveHandler := liveHandler{
		logsGoroutines: options.LogsGoroutines,
		log:            log,
	}

	readyHandler := readyHandler{
		logsGoroutines: options.LogsGoroutines,
		log:            log,
	}

	r.Handle("/live", liveHandler)
	r.Handle("/ready", readyHandler)
	// Default route for backward compatibility
	r.NewRoute().Handler(liveHandler)

	return &http.Server{
		Handler:           r,
		ReadTimeout:       defaultTimeout,
		ReadHeaderTimeout: defaultTimeout,
		WriteTimeout:      defaultTimeout,
	}
}

func healthHandler(logsGoroutines bool, log log.Component, getStatusNonBlocking func() (health.Status, error), w http.ResponseWriter, _ *http.Request) {
	health, err := getStatusNonBlocking()
	if err != nil {
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), http.StatusInternalServerError)
		return
	}

	if len(health.Unhealthy) > 0 {
		w.WriteHeader(http.StatusInternalServerError)
		log.Infof("Healthcheck failed on: %v", health.Unhealthy)
		if logsGoroutines {
			log.Infof("Goroutines stack: \n%s\n", allStack())
		}
	}

	jsonHealth, err := json.Marshal(health)
	if err != nil {
		log.Errorf("Error marshalling status. Error: %v", err)
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), 500)
		return
	}

	w.Write(jsonHealth)
}

// inspired by https://golang.org/src/runtime/debug/stack.go?s=587:606#L11
func allStack() []byte {
	buf := make([]byte, 1024)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return buf[:n]
		}
		buf = make([]byte, 2*len(buf))
	}
}
