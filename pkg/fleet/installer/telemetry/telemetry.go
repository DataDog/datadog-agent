// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package telemetry provides the telemetry for fleet components.
package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	envTraceID          = "DATADOG_TRACE_ID"
	envParentID         = "DATADOG_PARENT_ID"
	envService          = "DATADOG_SERVICE"
	envSamplingPriority = "DATADOG_SAMPLING_PRIORITY"
	telemetrySubdomain  = "instrumentation-telemetry-intake"
)

// reGovCloudSite matches GovCloud sites (e.g. "ddog-gov.com", "xxxx99.ddog-gov.com").
// No instrumentation telemetry intake backend exists on GovCloud, so telemetry is
// disabled outright for these sites rather than attempting (and failing) to send.
// This mirrors the equivalent exclusion for the resident agent's own telemetry in
// pkg/config/utils/telemetry.go's IsAgentTelemetryEnabled.
// Expects a normalized (lowercase, no trailing DNS root dot) site — see normalizeSite.
var reGovCloudSite = regexp.MustCompile(`^(?:.+\.)?ddog-gov\.com$`)

func normalizeSite(site string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(site)), ".")
}

// Telemetry handles the telemetry for fleet components.
type Telemetry struct {
	telemetryClient  *client
	done             chan struct{}
	flushed          chan struct{}
	flushInterval    time.Duration
	stopFlushTimeout time.Duration
	sendCtx          context.Context
	cancelSends      context.CancelFunc

	env     string
	service string
}

type options struct {
	endpointURL      string
	disabled         bool
	stopFlushTimeout time.Duration
}

// Option configures a Telemetry instance.
type Option func(*options)

// WithEndpointURL sends telemetry to url, e.g. apm_config.telemetry.dd_url, instead of the
// site's instrumentation telemetry intake.
func WithEndpointURL(url string) Option {
	return func(o *options) { o.endpointURL = url }
}

// WithDisabled turns off sending telemetry.
func WithDisabled() Option {
	return func(o *options) { o.disabled = true }
}

// WithStopFlushTimeout bounds how long Stop waits for sends to finish, both a periodic send
// already in flight and the final flush, so an unreachable intake cannot hold up process
// shutdown for the full send timeout.
func WithStopFlushTimeout(timeout time.Duration) Option {
	return func(o *options) { o.stopFlushTimeout = timeout }
}

// NewTelemetry creates a new telemetry instance
func NewTelemetry(client *http.Client, apiKey string, site string, service string, opts ...Option) *Telemetry {
	t := newTelemetry(client, apiKey, site, service, opts...)
	t.Start()
	return t
}

func newTelemetry(client *http.Client, apiKey string, site string, service string, opts ...Option) *Telemetry {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	if site == "" {
		site = "datadoghq.com"
	}
	host := fmt.Sprintf("https://%s.%s", telemetrySubdomain, strings.TrimSpace(site))
	if url := strings.TrimSpace(o.endpointURL); url != "" {
		host = strings.TrimSuffix(url, "/")
	}
	e := &endpoint{
		Host:   host,
		APIKey: apiKey,
	}
	env := "prod"
	if site == "datad0g.com" {
		env = "staging"
	}

	siteSupportsTelemetry := !reGovCloudSite.MatchString(normalizeSite(site))
	if !siteSupportsTelemetry {
		log.Debugf("telemetry disabled: no instrumentation telemetry intake exists for GovCloud site %q", site)
	}
	if o.disabled {
		log.Debugf("telemetry disabled by configuration")
	}

	sendCtx, cancelSends := context.WithCancel(context.Background())
	return &Telemetry{
		telemetryClient:  newClient(client, []*endpoint{e}, service, site == "datad0g.com", siteSupportsTelemetry && !o.disabled),
		done:             make(chan struct{}),
		flushed:          make(chan struct{}),
		flushInterval:    time.Minute,
		stopFlushTimeout: o.stopFlushTimeout,
		sendCtx:          sendCtx,
		cancelSends:      cancelSends,
		env:              env,
		service:          service,
	}
}

// Start starts the telemetry
func (t *Telemetry) Start() {
	ticker := time.Tick(t.flushInterval)
	go func() {
		for {
			select {
			case <-ticker:
				t.sendCompletedSpans(t.sendCtx)
			case <-t.done:
				t.sendCompletedSpans(t.sendCtx)
				close(t.flushed)
				return
			}
		}
	}()
}

// Stop stops the telemetry
func (t *Telemetry) Stop() {
	if t.stopFlushTimeout > 0 {
		deadline := time.AfterFunc(t.stopFlushTimeout, t.cancelSends)
		defer deadline.Stop()
	}
	close(t.done)
	<-t.flushed
	t.cancelSends()
}

func (t *Telemetry) extractCompletedSpans() traces {
	spans := globalTracer.flushCompletedSpans()
	if len(spans) == 0 {
		return traces{}
	}
	ts := make(map[uint64][]*span)
	for _, span := range spans {
		if span.span.Service == "" {
			span.span.Service = t.service
		}
		span.span.Meta["env"] = t.env
		span.span.Meta["version"] = version.AgentVersion
		if _, ok := span.span.Metrics["_sampling_priority_v1"]; !ok {
			span.span.Metrics["_sampling_priority_v1"] = 2
		}
		ts[span.span.TraceID] = append(ts[span.span.TraceID], &span.span)
	}
	tracesArray := make([]trace, 0, len(ts))
	for _, t := range ts {
		tracesArray = append(tracesArray, trace(t))
	}
	return traces(tracesArray)
}

func (t *Telemetry) sendCompletedSpans(ctx context.Context) {
	tracesArray := t.extractCompletedSpans()
	if len(tracesArray) == 0 {
		return
	}
	t.telemetryClient.sendTraces(ctx, tracesArray)
}

// SpanFromContext returns the span from the context if available.
func SpanFromContext(ctx context.Context) (*Span, bool) {
	spanIDs, ok := getSpanIDsFromContext(ctx)
	if !ok {
		return nil, false
	}
	return globalTracer.getSpan(spanIDs.spanID)
}

// WithService sets the service name on the context. Spans created from this context
// (and child contexts) inherit the service unless overridden by another WithService call.
func WithService(ctx context.Context, service string) context.Context {
	return context.WithValue(ctx, serviceKey, service)
}

// WithSamplingPriority sets the sampling priority on the context. Spans created from
// this context (and child contexts) inherit the priority unless overridden by another
// WithSamplingPriority call. When unset, a default of 2 (FORCE_KEEP) is stamped at flush.
func WithSamplingPriority(ctx context.Context, priority int) context.Context {
	return context.WithValue(ctx, samplingPriorityKey, priority)
}

func getServiceFromContext(ctx context.Context) (string, bool) {
	service, ok := ctx.Value(serviceKey).(string)
	return service, ok
}

func getSamplingPriorityFromContext(ctx context.Context) (*int, bool) {
	priority, ok := ctx.Value(samplingPriorityKey).(int)
	if !ok {
		return nil, false
	}
	return &priority, true
}

// StartSpanFromEnv starts a span using the environment variables to find the parent span.
// It also reads DATADOG_SERVICE and DATADOG_SAMPLING_PRIORITY from the environment and
// applies them to the context so the created span (and its children) inherit them.
func StartSpanFromEnv(ctx context.Context, operationName string) (*Span, context.Context) {
	if service, ok := os.LookupEnv(envService); ok {
		ctx = WithService(ctx, service)
	}
	if priorityStr, ok := os.LookupEnv(envSamplingPriority); ok {
		if priority, err := strconv.Atoi(priorityStr); err == nil {
			ctx = WithSamplingPriority(ctx, priority)
		}
	}
	traceID, parentID := extractIDsFromEnv()
	return StartSpanFromIDs(ctx, operationName, traceID, parentID)
}

func extractIDsFromEnv() (string, string) {
	parentID, ok := os.LookupEnv(envParentID)
	if !ok {
		return "0", "0"
	}
	traceID, ok := os.LookupEnv(envTraceID)
	if !ok {
		return "0", "0"
	}
	return traceID, parentID
}

func converIDsToUint64(traceID, parentID string) (uint64, uint64) {
	traceIDInt, err := strconv.ParseUint(traceID, 10, 64)
	if err != nil {
		return 0, 0
	}
	parentIDInt, err := strconv.ParseUint(parentID, 10, 64)
	if err != nil {
		return 0, 0
	}
	return traceIDInt, parentIDInt
}

// StartSpanFromIDs starts a span using the trace and parent IDs provided as
// decimal strings. Malformed inputs yield a fresh top-level trace.
func StartSpanFromIDs(ctx context.Context, operationName, traceID, parentID string) (*Span, context.Context) {
	traceIDInt, parentIDInt := converIDsToUint64(traceID, parentID)
	return StartSpanFromUint64IDs(ctx, operationName, traceIDInt, parentIDInt)
}

// StartSpanFromUint64IDs starts a span using the trace and parent IDs provided
// as uint64. The created span is marked top-level: it is the entry point of
// this process's participation in the trace.
func StartSpanFromUint64IDs(ctx context.Context, operationName string, traceID, parentID uint64) (*Span, context.Context) {
	span, ctx := startSpanFromIDs(ctx, operationName, traceID, parentID)
	span.SetTopLevel()
	return span, ctx
}

func startSpanFromIDs(ctx context.Context, operationName string, traceID, parentID uint64) (*Span, context.Context) {
	service, _ := getServiceFromContext(ctx)
	samplingPriority, _ := getSamplingPriorityFromContext(ctx)
	s := newSpan(operationName, parentID, traceID, service, samplingPriority)
	ctx = setSpanIDsInContext(ctx, s)
	return s, ctx
}

// StartSpanFromContext starts a span using the context to find the parent span.
func StartSpanFromContext(ctx context.Context, operationName string) (*Span, context.Context) {
	spanIDs, _ := getSpanIDsFromContext(ctx)
	return startSpanFromIDs(ctx, operationName, spanIDs.traceID, spanIDs.spanID)
}

// EnvFromContext returns the environment variables for the context.
// Service and sampling priority are included when set on the context so child
// processes can inherit them via StartSpanFromEnv.
func EnvFromContext(ctx context.Context) []string {
	sIDs, ok := getSpanIDsFromContext(ctx)
	if !ok {
		return []string{}
	}
	env := []string{
		fmt.Sprintf("%s=%s", envTraceID, strconv.FormatUint(sIDs.traceID, 10)),
		fmt.Sprintf("%s=%s", envParentID, strconv.FormatUint(sIDs.spanID, 10)),
	}
	if service, ok := getServiceFromContext(ctx); ok && service != "" {
		env = append(env, fmt.Sprintf("%s=%s", envService, service))
	}
	if priority, ok := getSamplingPriorityFromContext(ctx); ok {
		env = append(env, fmt.Sprintf("%s=%d", envSamplingPriority, *priority))
	}
	return env
}
