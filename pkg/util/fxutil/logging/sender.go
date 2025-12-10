// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// sendSpansToDatadog sends an array of TraceSpan to the local trace-agent.
// It uses the v0.3/traces endpoint with JSON format (array of traces).
func sendSpansToDatadog(agentLogger io.Writer, spans []*Span, traceAgentPort string) {
	// v0.3 format: array of traces, where each trace is an array of spans
	// We put all spans into a single trace
	traces := [][]*Span{spans}

	// Encode as JSON
	data, err := json.Marshal(traces)
	if err != nil {
		fmt.Fprintf(agentLogger, "[Fx Tracing] Failed to marshal traces to JSON: %v\n", err) //nolint:errcheck
		return
	}

	fmt.Fprintf(agentLogger, "[Fx Tracing] Encoded %d spans into %d bytes of JSON data\n", len(spans), len(data)) //nolint:errcheck

	// Send to trace-agent with retries
	sendWithRetries(agentLogger, data, traceAgentPort, len(traces), 2*time.Second, 10*time.Second)
}

// sendWithRetries sends the JSON payload to the trace-agent with retry logic.
func sendWithRetries(agentLogger io.Writer, data []byte, traceAgentPort string, traceCount int, retryInterval time.Duration, timeout time.Duration) {
	// Use v0.3/traces endpoint (JSON format)
	agentURL := "http://localhost:" + traceAgentPort + "/v0.3/traces"
	client := &http.Client{Timeout: retryInterval}

	// Wait forever, periodically refreshing our registration.
	retryTicker := time.NewTicker(retryInterval)
	attempt := 0
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			fmt.Printf("[Fx Tracing] Failed to send %d spans to Datadog after %d attempts\n", traceCount, attempt) //nolint:errcheck
			return
		case <-retryTicker.C:
			attempt++
			req, err := createRequest(agentURL, data, traceCount)
			if err != nil {
				fmt.Fprintf(agentLogger, "[Fx Tracing] Failed to create request: %v\n", err) //nolint:errcheck
				continue
			}

			resp, err := client.Do(req)
			if err != nil {
				fmt.Fprintf(agentLogger, "[Fx Tracing] Failed to send request: %v\n", err) //nolint:errcheck
				continue
			}

			resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				fmt.Fprintf(agentLogger, "[Fx Tracing] Unexpected status code %d (attempt %d): %s\n", resp.StatusCode, attempt, resp.Status) //nolint:errcheck
				continue
			}

			fmt.Fprintf(agentLogger, "[Fx Tracing] Successfully sent %d spans to Datadog\n", traceCount) //nolint:errcheck
			return
		}
	}
}

func createRequest(agentURL string, data []byte, traceCount int) (*http.Request, error) {
	req, err := http.NewRequest("PUT", agentURL, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	// Following the same logic as the dd-trace-go tracer by https://github.com/DataDog/dd-trace-go/blob/0268bdb68c5abdf91ab210fdd46bcab64c814964/ddtrace/tracer/transport.go#L73-L98
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Datadog-Meta-Lang", "go")
	req.Header.Set("Datadog-Meta-Lang-Version", strings.TrimPrefix(runtime.Version(), "go"))
	req.Header.Set("X-Datadog-Trace-Count", strconv.Itoa(traceCount))
	return req, nil
}
