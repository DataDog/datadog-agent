// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"

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

// Telemetry handles the installer telemetry
type Telemetry struct {
	telemetryClient internaltelemetry.Client

	site string

	listener *telemetryListener
	server   *http.Server
	client   *http.Client
}

// NewTelemetry creates a new telemetry instance
func NewTelemetry(config config.Reader) (*Telemetry, error) {
	endpoint := &traceconfig.Endpoint{
		Host:   utils.GetMainEndpoint(config, traceconfig.TelemetryEndpointPrefix, "updater.telemetry.dd_url"),
		APIKey: utils.SanitizeAPIKey(config.GetString("api_key")),
	}
	site := config.GetString("site")
	listener := newTelemetryListener()
	t := &Telemetry{
		telemetryClient: internaltelemetry.NewClient(http.DefaultClient, []*traceconfig.Endpoint{endpoint}, "datadog-installer", site == "datad0g.com"),
		site:            site,
		listener:        listener,
		server:          &http.Server{},
		client: &http.Client{
			Transport: &http.Transport{
				Dial: listener.Dial,
			},
		},
	}
	t.server.Handler = t.handler()
	return t, nil
}

// Start starts the telemetry
func (t *Telemetry) Start(_ context.Context) error {
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
		tracer.WithHTTPClient(t.client),
		tracer.WithLogStartup(false),
	)
	return nil
}

// Stop stops the telemetry
func (t *Telemetry) Stop(ctx context.Context) error {
	tracer.Flush()
	tracer.Stop()
	t.listener.Close()
	err := t.server.Shutdown(ctx)
	if err != nil {
		log.Errorf("error shutting down telemetry server: %v", err)
	}
	return nil
}

func (t *Telemetry) handler() http.Handler {
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
		t.telemetryClient.SendTraces(traces)
		w.WriteHeader(http.StatusOK)
	})
	return r
}

type telemetryListener struct {
	conns chan net.Conn

	close     chan struct{}
	closeOnce sync.Once
}

func newTelemetryListener() *telemetryListener {
	return &telemetryListener{
		conns: make(chan net.Conn),
		close: make(chan struct{}),
	}
}

func (l *telemetryListener) Close() error {
	l.closeOnce.Do(func() {
		close(l.close)
	})
	return nil
}

func (l *telemetryListener) Accept() (net.Conn, error) {
	select {
	case <-l.close:
		return nil, errors.New("listener closed")
	case conn := <-l.conns:
		return conn, nil
	}
}

func (l *telemetryListener) Addr() net.Addr {
	return addr(0)
}

func (l *telemetryListener) Dial(_, _ string) (net.Conn, error) {
	select {
	case <-l.close:
		return nil, errors.New("listener closed")
	default:
	}
	server, client := net.Pipe()
	l.conns <- server
	return client, nil
}

type addr int

func (addr) Network() string {
	return "memory"
}

func (addr) String() string {
	return "local"
}
