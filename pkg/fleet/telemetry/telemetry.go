// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package telemetry provides the telemetry for fleet components.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/internaltelemetry"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// EnvTraceID is the environment variable key for the trace ID
	EnvTraceID = "DATADOG_TRACE_ID"
	// EnvParentID is the environment variable key for the parent ID
	EnvParentID = "DATADOG_PARENT_ID"
)

const (
	telemetrySubdomain = "instrumentation-telemetry-intake"
	telemetryEndpoint  = "/v0.4/traces"
)

// Telemetry handles the telemetry for fleet components.
type Telemetry struct {
	telemetryClient internaltelemetry.Client

	site    string
	service string

	listener *telemetryListener
	server   *http.Server
	client   *http.Client
}

// NewTelemetry creates a new telemetry instance
func NewTelemetry(apiKey string, site string, service string) (*Telemetry, error) {
	endpoint := &traceconfig.Endpoint{
		Host:   fmt.Sprintf("https://%s.%s", telemetrySubdomain, strings.TrimSpace(site)),
		APIKey: apiKey,
	}
	listener := newTelemetryListener()
	t := &Telemetry{
		telemetryClient: internaltelemetry.NewClient(http.DefaultClient, []*traceconfig.Endpoint{endpoint}, service, site == "datad0g.com"),
		site:            site,
		service:         service,
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
		tracer.WithServiceName(t.service),
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

// SpanContextFromEnv injects the traceID and parentID from the environment into the context if available.
func SpanContextFromEnv() (ddtrace.SpanContext, bool) {
	traceID := os.Getenv(EnvTraceID)
	parentID := os.Getenv(EnvParentID)
	ctxCarrier := tracer.TextMapCarrier{
		tracer.DefaultTraceIDHeader:  traceID,
		tracer.DefaultParentIDHeader: parentID,
		tracer.DefaultPriorityHeader: "2",
	}
	spanCtx, err := tracer.Extract(ctxCarrier)
	if err != nil {
		log.Debugf("failed to extract span context from install script params: %v", err)
		return nil, false
	}
	return spanCtx, true
}

// EnvFromSpanContext returns the environment variables for the span context.
func EnvFromSpanContext(spanCtx ddtrace.SpanContext) []string {
	env := []string{
		fmt.Sprintf("%s=%d", EnvTraceID, spanCtx.TraceID()),
		fmt.Sprintf("%s=%d", EnvParentID, spanCtx.SpanID()),
	}
	return env
}

// SpanContextFromContext extracts the span context from the context if available.
func SpanContextFromContext(ctx context.Context) (ddtrace.SpanContext, bool) {
	span, ok := tracer.SpanFromContext(ctx)
	if !ok {
		return nil, false
	}
	return span.Context(), true
}
