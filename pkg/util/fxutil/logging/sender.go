// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

// Span represents a Datadog APM span in JSON format for the v0.3/traces endpoint.
type Span struct {
	Service  string             `json:"service"`
	Name     string             `json:"name"`
	Resource string             `json:"resource"`
	TraceID  uint64             `json:"trace_id"`
	SpanID   uint64             `json:"span_id"`
	ParentID uint64             `json:"parent_id"`
	Start    int64              `json:"start"`
	Duration int64              `json:"duration"`
	Error    int32              `json:"error"`
	Meta     map[string]string  `json:"meta,omitempty"`
	Metrics  map[string]float64 `json:"metrics,omitempty"`
	Type     string             `json:"type,omitempty"`
}

// sendSpansToDatadog sends an array of TraceSpan to the local trace-agent.
// It uses the v0.3/traces endpoint with JSON format (array of traces).
func (l *fxTracingLogger) sendSpansToDatadog(flavor string) {
	l.mu.Lock()

	globalMeta := map[string]string{"flavor": flavor}

	// Generate a single trace ID for all spans
	traceID := rand.Uint64()

	// Generate span IDs for hierarchy
	appSpanID := rand.Uint64()
	constructorsSpanID := rand.Uint64()
	onStartHooksSpanID := rand.Uint64()

	// isErr := l.err != nil

	// Create the parent lifecycle span
	appSpan := &Span{
		Service:  serviceName,
		Name:     "Fx Application Startup",
		Resource: "Fx Application Startup",
		TraceID:  traceID,
		SpanID:   appSpanID,
		ParentID: 0,
		Start:    l.startTime.UnixNano(),
		Duration: l.endTime.Sub(l.startTime).Nanoseconds(),
		Error:    errToCode(nil),
		Type:     "custom",
		Meta:     globalMeta,
	}

	// Create constructors aggregator span if we have constructors
	constructorsAggSpan := &Span{
		Service:  serviceName,
		Name:     "Fx Constructors",
		Resource: "Fx Constructors",
		TraceID:  traceID,
		SpanID:   constructorsSpanID,
		ParentID: appSpanID,
		Start:    l.startTime.UnixNano(),
		Duration: l.lifecycleStart.Sub(l.startTime).Nanoseconds(),
		Type:     "custom",
		Meta:     globalMeta,
	}

	// Create onstart hooks aggregator span if we have onstart hooks
	onStartAggSpan := &Span{
		Service:  serviceName,
		Name:     "Fx OnStart Hooks",
		Resource: "Fx OnStart Hooks",
		TraceID:  traceID,
		SpanID:   onStartHooksSpanID,
		ParentID: appSpanID,
		Start:    l.lifecycleStart.UnixNano(),
		Duration: l.endTime.Sub(l.lifecycleStart).Nanoseconds(),
		Type:     "custom",
		Meta:     globalMeta,
	}
	spans := make([]*Span, 0, len(l.spans)+2)
	spans = append(spans, appSpan, constructorsAggSpan, onStartAggSpan)

	// Convert TraceSpans to JSON Spans
	for _, ts := range l.spans {
		span := convertToSpan(ts, traceID, constructorsSpanID, onStartHooksSpanID, globalMeta)
		spans = append(spans, span)
	}
	l.mu.Unlock()

	// v0.3 format: array of traces, where each trace is an array of spans
	// We put all spans into a single trace
	traces := [][]*Span{spans}

	// Encode as JSON
	data, err := json.Marshal(traces)
	if err != nil {
		l.agentLogger.Errorf("[Fx Tracing] Failed to marshal traces to JSON: %v", err) //nolint:errcheck
		return
	}

	l.agentLogger.Infof("[Fx Tracing] Encoded %d spans into %d bytes of JSON data", len(spans), len(data))

	// Send to trace-agent with retries
	sendWithRetries(l.agentLogger, data, len(traces), 5, 2*time.Second)
}

// convertToSpan converts a TraceSpan to a JSON Span.
func convertToSpan(ts *TraceSpan, traceID, constructorsSpanID, onStartHooksSpanID uint64, globalMeta map[string]string) *Span {
	var name string
	var parentID uint64
	switch ts.Type {
	case fxConstructorType:
		name = "Fx Constructor"
		parentID = constructorsSpanID
	case fxOnStartHookType:
		name = "Fx OnStart Hook"
		parentID = onStartHooksSpanID
	}

	// Create the span
	span := &Span{
		Service:  serviceName,
		Name:     name,
		Resource: ts.Resource,
		TraceID:  traceID,
		SpanID:   rand.Uint64(),
		ParentID: parentID,
		Start:    ts.Start,
		Duration: ts.Duration,
		Error:    errToCode(ts.Error),
		Type:     "custom",
		Meta:     globalMeta,
	}

	return span
}

func errToCode(err error) int32 {
	if err != nil {
		return 1
	}
	return 0
}

// sendWithRetries sends the JSON payload to the trace-agent with retry logic.
func sendWithRetries(logger Logger, data []byte, traceCount int, maxRetries int, retryInterval time.Duration) {

	// We can't rely on the trace agent URL from the config because it's not set in the fxutil package.
	// But we can still check for env variable override to set the trace agent URL.
	traceAgentPort := os.Getenv("DD_APM_RECEIVER_PORT")
	if traceAgentPort == "" {
		traceAgentPort = "8126"
	}
	// Use v0.3/traces endpoint (JSON format)
	agentURL := "http://localhost:" + traceAgentPort + "/v0.3/traces"

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryInterval)
		}

		req, err := http.NewRequest("PUT", agentURL, bytes.NewReader(data))
		if err != nil {
			logger.Debugf("[Fx Tracing] Failed to create request: %v", err)
			continue
		}

		// Set headers for JSON format
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Datadog-Meta-Lang", "go")
		req.Header.Set("Datadog-Meta-Lang-Version", strings.TrimPrefix(runtime.Version(), "go"))
		req.Header.Set("X-Datadog-Trace-Count", fmt.Sprintf("%d", traceCount))

		// Send request
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			logger.Debugf("[Fx Tracing] Failed to send request (attempt %d/%d): %v", attempt+1, maxRetries+1, err)
			continue
		}

		// Check response
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			logger.Infof("[Fx Tracing] Successfully sent %d spans to Datadog (service: %s)", traceCount, serviceName)
			return
		}

		logger.Debugf("[Fx Tracing] Unexpected status code %d (attempt %d/%d): %s", resp.StatusCode, attempt+1, maxRetries+1, string(body))
	}

	logger.Errorf("[Fx Tracing] Failed after %d retries", maxRetries) //nolint:errcheck
}
