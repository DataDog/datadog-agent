// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/internaltelemetry"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	telemetryEndpoint = "/v0.4/traces"
)

type telemetry struct {
	client internaltelemetry.Client

	site       string
	socketPath string
	listener   net.Listener
	server     *http.Server
}

func newTelemetry(config config.Reader, socketDirectory string) (*telemetry, error) {
	// HACK: we use a unix socket to receive traces from the local tracer
	// this isn't ideal and could be replaced by an in-memory transport in the future
	socketPath := filepath.Join(socketDirectory, "telemetry.sock")
	err := os.RemoveAll(socketPath)
	if err != nil {
		return nil, fmt.Errorf("could not remove socket: %w", err)
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socketPath, 0700); err != nil {
		return nil, fmt.Errorf("error setting socket permissions: %v", err)
	}
	endpoint := &traceconfig.Endpoint{
		Host:   utils.GetMainEndpoint(config, traceconfig.TelemetryEndpointPrefix, "updater.telemetry.dd_url"),
		APIKey: utils.SanitizeAPIKey(config.GetString("api_key")),
	}
	site := config.GetString("site")
	t := &telemetry{
		client:     internaltelemetry.NewClient(http.DefaultClient, []*traceconfig.Endpoint{endpoint}, "datadog-installer", site == "datad0g.com"),
		site:       site,
		socketPath: socketPath,
		listener:   listener,
		server:     &http.Server{},
	}
	t.server.Handler = t.handler()
	return t, nil
}

// Start starts the telemetry
func (t *telemetry) Start(_ context.Context) {
	go func() {
		err := t.server.Serve(t.listener)
		if err != nil {
			log.Infof("telemetry server stopped: %v", err)
		}
	}()
	env := "prod"
	if t.site == "datad0g.com" {
		env = "staging"
	}
	tracer.Start(
		tracer.WithServiceName("datadog-installer"),
		tracer.WithServiceVersion(version.AgentVersion),
		tracer.WithEnv(env),
		tracer.WithGlobalTag("site", t.site),
		tracer.WithUDS(t.socketPath),
	)
}

// Stop stops the telemetry
func (t *telemetry) Stop(ctx context.Context) {
	tracer.Flush()
	tracer.Stop()
	t.listener.Close()
	err := t.server.Shutdown(ctx)
	if err != nil {
		log.Errorf("error shutting down telemetry server: %v", err)
	}
}

func (t *telemetry) handler() http.Handler {
	r := mux.NewRouter().Headers("Content-Type", "application/msgpack").Subrouter()
	r.HandleFunc(telemetryEndpoint, func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Errorf("error reading request body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var traces pb.Traces
		_, err = traces.UnmarshalMsg(body)
		if err != nil {
			log.Errorf("error unmarshalling traces: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		t.client.SendTraces(traces)
		w.WriteHeader(http.StatusOK)
	})
	return r
}
