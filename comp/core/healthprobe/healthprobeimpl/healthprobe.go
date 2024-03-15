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

	"github.com/DataDog/datadog-agent/comp/core/config"
	healthprobeComp "github.com/DataDog/datadog-agent/comp/core/healthprobe"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
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

	Log    log.Component
	Config config.Component
}

type healthprobe struct {
	config config.Component
	log    log.Component
	port   int
}

func (h *healthprobe) Serve(ctx context.Context) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%v", h.port))
	if err != nil {
		return err
	}

	r := mux.NewRouter()
	r.HandleFunc("/live", h.liveHandler())
	r.HandleFunc("/ready", h.readyHandler())
	// Default route for backward compatibility
	r.NewRoute().HandlerFunc(h.liveHandler())

	srv := &http.Server{
		Handler:           r,
		ReadTimeout:       defaultTimeout,
		ReadHeaderTimeout: defaultTimeout,
		WriteTimeout:      defaultTimeout,
	}

	go srv.Serve(ln) //nolint:errcheck
	go closeOnContext(ctx, srv)
	return nil
}

func closeOnContext(ctx context.Context, srv *http.Server) {
	// Wait for the context to be canceled
	<-ctx.Done()

	// Shutdown the server, it will close the listener
	timeout, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	srv.Shutdown(timeout) //nolint:errcheck
}

func (h *healthprobe) liveHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		h.healthHandler(health.GetLiveNonBlocking, w, r)
	}
}

func (h *healthprobe) readyHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		h.healthHandler(health.GetReadyNonBlocking, w, r)
	}
}

func (h *healthprobe) healthHandler(getStatusNonBlocking func() (health.Status, error), w http.ResponseWriter, _ *http.Request) {
	health, err := getStatusNonBlocking()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	if len(health.Unhealthy) > 0 {
		w.WriteHeader(http.StatusInternalServerError)
		h.log.Infof("Healthcheck failed on: %v", health.Unhealthy)
		if h.config.GetBool("log_all_goroutines_when_unhealthy") {
			h.log.Infof("Goroutines stack: \n%s\n", allStack())
		}
	}

	jsonHealth, err := json.Marshal(health)
	if err != nil {
		h.log.Errorf("Error marshalling status. Error: %v", err)
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

func newHealthProbe(deps dependencies) optional.Option[healthprobeComp.Component] {
	healthPort := deps.Config.GetInt("health_port")
	if healthPort <= 0 {
		return optional.NewNoneOption[healthprobeComp.Component]()
	}

	probe := &healthprobe{
		config: deps.Config,
		log:    deps.Log,
		port:   healthPort,
	}

	return optional.NewOption[healthprobeComp.Component](probe)
}
