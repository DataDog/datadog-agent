// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package mode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	serverlessLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// PubSubMessage represents a Pub/Sub message structure
type PubSubMessage struct {
	Message struct {
		Data       string            `json:"data"`
		Attributes map[string]string `json:"attributes"`
		MessageID  string            `json:"messageId"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

// CloudRunProxyConfig holds configuration for the Cloud Run proxy server
type CloudRunProxyConfig struct {
	// No config needed - use defaults
}

// CloudRunProxy represents the HTTP server for Cloud Run proxy
type CloudRunProxy struct {
	config *CloudRunProxyConfig
	client *http.Client
}

// RunCloudRunProxy starts the Cloud Run proxy HTTP server
func RunCloudRunProxy(_ *serverlessLog.Config) error {
	// Use DD_HEALTH_PORT or default to 443
	proxyPort := "443"
	if port := os.Getenv("DD_HEALTH_PORT"); port != "" {
		proxyPort = port
	}

	// Create HTTP server with optimized client
	mux := http.NewServeMux()
	proxy := &CloudRunProxy{
		config: &CloudRunProxyConfig{},
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				DisableKeepAlives:   false,
				MaxConnsPerHost:     20,
			},
		},
	}

	mux.HandleFunc("/", proxy.handleRequest)

	server := &http.Server{
		Addr:    ":" + proxyPort,
		Handler: mux,
	}

	// Start server in goroutine
	go func() {
		log.Infof("Starting Cloud Run proxy server on port %s, forwarding to localhost:8080", proxyPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorf("Cloud Run proxy server error: %v", err)
		}
	}()

	// Handle shutdown signals
	stopCh := make(chan struct{})
	go func() {
		signalCh := make(chan os.Signal, 1)
		signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
		<-signalCh
		log.Info("Shutting down Cloud Run proxy server...")
		stopCh <- struct{}{}
	}()

	<-stopCh

	// Graceful shutdown with default timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Errorf("Error shutting down proxy server: %v", err)
	}

	return nil
}

// handleRequest processes incoming HTTP requests with optimized Pub/Sub detection and processing
func (p *CloudRunProxy) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Inline Pub/Sub detection for better performance
	contentType := r.Header.Get("Content-Type")
	userAgent := r.Header.Get("User-Agent")
	isPubSub := contentType == "application/json" && strings.Contains(userAgent, "APIs-Google")

	if isPubSub {
		log.Debugf("Cloud Run proxy: received Pub/Sub request to %s", r.URL.Path)

		// Read and parse Pub/Sub message
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Errorf("Cloud Run proxy: error reading request body: %v", err)
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		r.Body.Close()

		// Parse Pub/Sub message and extract trace context
		var pubsubMsg PubSubMessage
		if err := json.Unmarshal(body, &pubsubMsg); err == nil {
			log.Debugf("Cloud Run proxy: detected Pub/Sub message with %s", pubsubMsg.Message)
			// Process trace context and create synthetic spans
			p.processTraceContextAndSpansFromAttrs(r, pubsubMsg.Message.Attributes)
		}

		// Recreate the request body for forwarding
		r.Body = io.NopCloser(strings.NewReader(string(body)))
	} else {
		log.Debugf("Cloud Run proxy: forwarding non-Pub/Sub request to %s", r.URL.Path)
	}

	// Forward request to target service
	p.forwardRequest(w, r)
}

// processTraceContextAndSpansFromAttrs processes trace context from extracted attributes
func (p *CloudRunProxy) processTraceContextAndSpansFromAttrs(r *http.Request, attrs map[string]string) {
	if len(attrs) == 0 {
		return
	}

	// Extract parent span ID (Datadog format first, then W3C)
	var parentSpanID uint64
	if datadogParentID := attrs["x-datadog-parent-id"]; datadogParentID != "" {
		if id, err := strconv.ParseUint(datadogParentID, 10, 64); err == nil {
			parentSpanID = id
		}
	} else if traceParent := attrs["traceparent"]; traceParent != "" {
		if parts := strings.Split(traceParent, "-"); len(parts) >= 2 {
			if id, err := strconv.ParseUint(parts[1], 16, 64); err == nil {
				parentSpanID = id
			}
		}
	}

	// Create Pub/Sub synthetic span
	var pubsubSpan tracer.Span
	if parentSpanID > 0 {
		pubsubSpan = tracer.StartSpan("pubsub.message.delivery", tracer.WithSpanID(parentSpanID))
	} else {
		pubsubSpan = tracer.StartSpan("pubsub.message.delivery")
	}

	// Set Pub/Sub span tags efficiently
	pubsubSpan.SetTag("service.name", "google.pubsub")
	pubsubSpan.SetTag("service", "google.pubsub")
	pubsubSpan.SetTag("resource.name", attrs["subscription"])
	pubsubSpan.SetTag("operation.name", "pubsub.message.delivery")
	pubsubSpan.SetTag("message_id", attrs["messageId"])
	pubsubSpan.SetTag("subscription", attrs["subscription"])
	pubsubSpan.SetTag("component", "pubsub_infrastructure")
	pubsubSpan.SetTag("span.kind", "consumer")
	pubsubSpan.SetTag("cloud.provider", "gcp")
	pubsubSpan.SetTag("cloud.service", "pubsub")
	pubsubSpan.SetTag("messaging.system", "pubsub")
	pubsubSpan.SetTag("messaging.operation", "receive")
	pubsubSpan.Finish()

	// Create Datadog processing span if we have trace context
	if parentSpanID > 0 {
		ddSpan := tracer.StartSpan("datadog.trace.processing", tracer.WithSpanID(parentSpanID))
		ddSpan.SetTag("service.name", "datadog.agent")
		ddSpan.SetTag("service", "datadog.agent")
		ddSpan.SetTag("resource.name", "cloudrun.proxy")
		ddSpan.SetTag("operation.name", "datadog.trace.processing")
		ddSpan.SetTag("message_id", attrs["messageId"])
		ddSpan.SetTag("subscription", attrs["subscription"])
		ddSpan.SetTag("processing_type", "trace_context_extraction")
		ddSpan.SetTag("component", "cloudrun_proxy")
		ddSpan.SetTag("span.kind", "internal")
		ddSpan.SetTag("datadog.processing.step", "trace_context_injection")
		ddSpan.SetTag("cloud.provider", "gcp")
		ddSpan.SetTag("cloud.service", "cloud_run")
		ddSpan.Finish()
	}

	// Inject trace context headers efficiently
	if traceParent := attrs["traceparent"]; traceParent != "" {
		r.Header.Set("traceparent", traceParent)
	}
	if traceState := attrs["tracestate"]; traceState != "" {
		r.Header.Set("tracestate", traceState)
	}
	if datadogTraceID := attrs["x-datadog-trace-id"]; datadogTraceID != "" {
		r.Header.Set("x-datadog-trace-id", datadogTraceID)
	}
	if datadogParentID := attrs["x-datadog-parent-id"]; datadogParentID != "" {
		r.Header.Set("x-datadog-parent-id", datadogParentID)
	}
	if datadogSamplingPriority := attrs["x-datadog-sampling-priority"]; datadogSamplingPriority != "" {
		r.Header.Set("x-datadog-sampling-priority", datadogSamplingPriority)
	} else {
		r.Header.Set("x-datadog-sampling-priority", "1") // Default to sampled
	}
	if datadogOrigin := attrs["x-datadog-origin"]; datadogOrigin != "" {
		r.Header.Set("x-datadog-origin", datadogOrigin)
	}
	if cloudTraceContext := attrs["x-cloud-trace-context"]; cloudTraceContext != "" {
		r.Header.Set("x-cloud-trace-context", cloudTraceContext)
	}

	// Create W3C traceparent from Datadog headers if needed
	if datadogTraceID := attrs["x-datadog-trace-id"]; datadogTraceID != "" {
		if datadogParentID := attrs["x-datadog-parent-id"]; datadogParentID != "" && attrs["traceparent"] == "" {
			r.Header.Set("traceparent", fmt.Sprintf("%s-%s-01", datadogTraceID, datadogParentID))
		}
	}
}

// forwardRequest forwards the request to the target Cloud Run service with optimized error handling
func (p *CloudRunProxy) forwardRequest(w http.ResponseWriter, r *http.Request) {
	// Build target URL efficiently
	targetURL := "http://localhost:8080" + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	log.Debugf("Cloud Run proxy: forwarding request to %s", targetURL)

	// Create new request to target
	req, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		log.Errorf("Cloud Run proxy: error creating target request: %v", err)
		w.WriteHeader(http.StatusBadGateway)
		return
	}

	// Copy headers efficiently
	req.Header = make(http.Header, len(r.Header))
	for key, values := range r.Header {
		req.Header[key] = values
	}
	req.Host = "localhost:8080"

	// Make request to target service
	resp, err := p.client.Do(req)
	if err != nil {
		log.Errorf("Cloud Run proxy: error forwarding request: %v", err)
		if errors.Is(err, context.DeadlineExceeded) {
			w.WriteHeader(http.StatusGatewayTimeout)
		} else {
			w.WriteHeader(http.StatusBadGateway)
		}
		return
	}
	defer resp.Body.Close()

	// Copy response headers and body efficiently
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Errorf("Cloud Run proxy: error copying response body: %v", err)
	}

	log.Debugf("Cloud Run proxy: forwarded request completed with status %d", resp.StatusCode)
}
