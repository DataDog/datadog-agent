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
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	httptrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/net/http"
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

	samplingRules []tracer.SamplingRule
}

// Option is a functional option for telemetry.
type Option func(*Telemetry)

// NewTelemetry creates a new telemetry instance
func NewTelemetry(client *http.Client, apiKey string, site string, service string, opts ...Option) *Telemetry {
	endpoint := &traceconfig.Endpoint{
		Host:   fmt.Sprintf("https://%s.%s", telemetrySubdomain, strings.TrimSpace(site)),
		APIKey: apiKey,
	}
	listener := newTelemetryListener()
	t := &Telemetry{
		telemetryClient: internaltelemetry.NewClient(client, []*traceconfig.Endpoint{endpoint}, service, site == "datad0g.com"),
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
	for _, opt := range opts {
		opt(t)
	}
	t.server.Handler = t.handler()
	return t
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
		tracer.WithService(t.service),
		tracer.WithServiceVersion(version.AgentVersion),
		tracer.WithEnv(env),
		tracer.WithGlobalTag("site", t.site),
		tracer.WithHTTPClient(t.client),
		tracer.WithLogStartup(false),

		// We don't need the value, we just need to enforce that it's not
		// the default. If it is, then the tracer will try to use the socket
		// if it exists -- and it always exists for newer agents.
		// If the agent address is the socket, the tracer overrides WithHTTPClient to use it.
		tracer.WithAgentAddr("192.0.2.42:12345"), // 192.0.2.0/24 is reserved
		tracer.WithSamplingRules(t.samplingRules),
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

// StartSpanFromIDs starts a span using the trace and parent
// IDs provided.
func StartSpanFromIDs(ctx context.Context, operationName, traceID, parentID string, spanOptions ...ddtrace.StartSpanOption) (Span, context.Context) {
	ctxCarrier := tracer.TextMapCarrier{
		tracer.DefaultTraceIDHeader:  traceID,
		tracer.DefaultParentIDHeader: parentID,
		tracer.DefaultPriorityHeader: "2",
	}
	spanCtx, err := tracer.Extract(ctxCarrier)
	if err != nil {
		log.Debugf("failed to extract span context from install script params: %v", err)
		return Span{tracer.StartSpan("remote_request")}, ctx
	}
	spanOptions = append([]ddtrace.StartSpanOption{tracer.ChildOf(spanCtx)}, spanOptions...)

	return StartSpanFromContext(ctx, operationName, spanOptions...)
}

// SpanFromContext returns the span from the context if available.
func SpanFromContext(ctx context.Context) (Span, bool) {
	span, ok := tracer.SpanFromContext(ctx)
	if !ok {
		return Span{}, false
	}
	return Span{span}, true
}

// StartSpanFromContext starts a span using the context to find the parent span.
func StartSpanFromContext(ctx context.Context, operationName string, spanOptions ...ddtrace.StartSpanOption) (Span, context.Context) {
	span, ctx := tracer.StartSpanFromContext(ctx, operationName, spanOptions...)
	return Span{span}, ctx
}

// StartSpanFromEnv starts a span using the environment variables to find the parent span.
func StartSpanFromEnv(ctx context.Context, operationName string, spanOptions ...ddtrace.StartSpanOption) (Span, context.Context) {
	traceID, ok := os.LookupEnv(EnvTraceID)
	if !ok {
		traceID = strconv.FormatUint(rand.Uint64(), 10)
	}
	parentID, ok := os.LookupEnv(EnvParentID)
	if !ok {
		parentID = "0"
	}
	return StartSpanFromIDs(ctx, operationName, traceID, parentID, spanOptions...)
}

// EnvFromContext returns the environment variables for the context.
func EnvFromContext(ctx context.Context) []string {
	spanCtx, ok := SpanContextFromContext(ctx)
	if !ok {
		return []string{}
	}
	return []string{
		fmt.Sprintf("%s=%d", EnvTraceID, spanCtx.TraceID()),
		fmt.Sprintf("%s=%d", EnvParentID, spanCtx.SpanID()),
	}
}

// SpanContextFromContext extracts the span context from the context if available.
func SpanContextFromContext(ctx context.Context) (ddtrace.SpanContext, bool) {
	span, ok := tracer.SpanFromContext(ctx)
	if !ok {
		return nil, false
	}
	return span.Context(), true
}

// WithSamplingRules sets the sampling rules for the telemetry.
func WithSamplingRules(rules ...tracer.SamplingRule) Option {
	return func(t *Telemetry) {
		t.samplingRules = rules
	}
}

// WrapRoundTripper wraps the round tripper with the telemetry round tripper.
func WrapRoundTripper(rt http.RoundTripper) http.RoundTripper {
	return httptrace.WrapRoundTripper(rt)
}
