// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package healthprobe implements the health check server
package healthprobe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultTimeout = time.Second

// Serve configures and starts the http server for the health check.
// It returns an error if the setup failed, or runs the server in a goroutine.
// Stop the server by cancelling the passed context.
func Serve(ctx context.Context, config model.Reader, port int) error {
	if port == 0 {
		return errors.New("port should be non-zero")
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%v", port))
	if err != nil {
		return err
	}

	r := mux.NewRouter()
	r.HandleFunc("/live", liveHandler(config))
	r.HandleFunc("/ready", readyHandler(config))
	// Default route for backward compatibility
	r.NewRoute().HandlerFunc(liveHandler(config))

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

func healthHandler(config model.Reader, getStatusNonBlocking func() (health.Status, error), w http.ResponseWriter, _ *http.Request) {
	health, err := getStatusNonBlocking()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	if len(health.Unhealthy) > 0 {
		w.WriteHeader(http.StatusInternalServerError)
		log.Infof("Healthcheck failed on: %v", health.Unhealthy)
		if config.GetBool("log_all_goroutines_when_unhealthy") {
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

func liveHandler(config model.Reader) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		healthHandler(config, health.GetLiveNonBlocking, w, r)
	}
}

func readyHandler(config model.Reader) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		healthHandler(config, health.GetReadyNonBlocking, w, r)
	}
}
