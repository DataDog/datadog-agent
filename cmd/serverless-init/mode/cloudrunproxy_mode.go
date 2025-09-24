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
	"strings"
	"syscall"
	"time"

	serverlessLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	fmt.Printf("CLOUDRUN_PROXY: Starting Cloud Run proxy...\n")

	// Use DD_HEALTH_PORT or default to 443
	proxyPort := "8080"
	if port := os.Getenv("DD_HEALTH_PORT"); port != "" {
		proxyPort = port
	}

	fmt.Printf("CLOUDRUN_PROXY: Using port %s\n", proxyPort)

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
		fmt.Printf("CLOUDRUN_PROXY: Starting HTTP server on port %s\n", proxyPort)
		log.Infof("Starting Cloud Run proxy server on port %s, forwarding to localhost:8081", proxyPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("CLOUDRUN_PROXY: Server error: %v\n", err)
			log.Errorf("Cloud Run proxy server error: %v", err)
		}
	}()

	// Handle shutdown signals (sidecar functionality)
	stopCh := make(chan struct{})
	go func() {
		signalCh := make(chan os.Signal, 1)
		signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
		signo := <-signalCh
		log.Infof("Received signal '%s', shutting down Cloud Run proxy server...", signo)
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
	fmt.Printf("CLOUDRUN_PROXY: Received request - Method: %s, URL: %s\n", r.Method, r.URL.String())
	fmt.Printf("CLOUDRUN_PROXY: Headers: %+v\n", r.Header)

	// Inline Pub/Sub detection for better performance
	contentType := r.Header.Get("Content-Type")
	userAgent := r.Header.Get("User-Agent")
	isPubSub := contentType == "application/json" && strings.Contains(userAgent, "APIs-Google")

	fmt.Printf("CLOUDRUN_PROXY: Content-Type: %s, User-Agent: %s\n", contentType, userAgent)
	fmt.Printf("CLOUDRUN_PROXY: Is Pub/Sub request: %v\n", isPubSub)

	if isPubSub {
		fmt.Printf("CLOUDRUN_PROXY: Processing Pub/Sub request to %s\n", r.URL.Path)
		log.Debugf("Cloud Run proxy: received Pub/Sub request to %s", r.URL.Path)

		// Read and parse Pub/Sub message
		body, err := io.ReadAll(r.Body)
		if err != nil {
			fmt.Printf("CLOUDRUN_PROXY: Error reading request body: %v\n", err)
			log.Errorf("Cloud Run proxy: error reading request body: %v", err)
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		r.Body.Close()

		fmt.Printf("CLOUDRUN_PROXY: Request body length: %d bytes\n", len(body))
		fmt.Printf("CLOUDRUN_PROXY: Request body preview: %s\n", string(body[:min(len(body), 500)]))

		// Parse Pub/Sub message and extract trace context
		var pubsubMsg PubSubMessage
		if err := json.Unmarshal(body, &pubsubMsg); err == nil {
			fmt.Printf("CLOUDRUN_PROXY: Successfully parsed Pub/Sub message\n")
			fmt.Printf("CLOUDRUN_PROXY: Message attributes: %+v\n", pubsubMsg.Message.Attributes)
			log.Debugf("Cloud Run proxy: detected Pub/Sub message with %s", pubsubMsg.Message)
			// Process trace context and create synthetic spans
			p.processTraceContextAndSpansFromAttrs(r, pubsubMsg.Message.Attributes)
		} else {
			fmt.Printf("CLOUDRUN_PROXY: Failed to parse Pub/Sub message: %v\n", err)
		}

		// Recreate the request body for forwarding
		r.Body = io.NopCloser(strings.NewReader(string(body)))
	} else {
		fmt.Printf("CLOUDRUN_PROXY: Forwarding non-Pub/Sub request to %s\n", r.URL.Path)
		log.Debugf("Cloud Run proxy: forwarding non-Pub/Sub request to %s", r.URL.Path)
	}

	// Forward request to target service
	fmt.Printf("CLOUDRUN_PROXY: Forwarding request to main container\n")
	p.forwardRequest(w, r)
}

// processTraceContextAndSpansFromAttrs processes trace context from extracted attributes
func (p *CloudRunProxy) processTraceContextAndSpansFromAttrs(r *http.Request, attrs map[string]string) {
	if len(attrs) == 0 {
		return
	}

	// Log the attributes for debugging
	fmt.Printf("Pub/Sub message attributes: %+v\n", attrs)

	// Inject trace context headers efficiently - prioritize Google Cloud trace context
	if googTraceParent := attrs["googclient_traceparent"]; googTraceParent != "" {
		r.Header.Set("traceparent", googTraceParent)
	}
	if googTraceState := attrs["googclient_tracestate"]; googTraceState != "" {
		r.Header.Set("tracestate", googTraceState)
	}
	if cloudTraceContext := attrs["x-cloud-trace-context"]; cloudTraceContext != "" {
		r.Header.Set("x-cloud-trace-context", cloudTraceContext)
	}

	// Fallback to standard trace context if Google Cloud context not available
	if r.Header.Get("traceparent") == "" {
		if traceParent := attrs["traceparent"]; traceParent != "" {
			r.Header.Set("traceparent", traceParent)
		}
	}
	if r.Header.Get("tracestate") == "" {
		if traceState := attrs["tracestate"]; traceState != "" {
			r.Header.Set("tracestate", traceState)
		}
	}

	// Extract and inject Datadog trace context from tracestate
	if tracestate := r.Header.Get("tracestate"); tracestate != "" {
		// Parse Datadog trace context from tracestate: dd=t.tid:TRACE_ID;s:SAMPLING;p:PARENT_ID
		if strings.Contains(tracestate, "dd=") {
			ddPart := strings.Split(tracestate, "dd=")[1]
			if strings.Contains(ddPart, ";") {
				ddPart = strings.Split(ddPart, ";")[0]
			}
			if strings.Contains(ddPart, "t.tid:") {
				ddTraceID := strings.Split(ddPart, "t.tid:")[1]
				r.Header.Set("x-datadog-trace-id", ddTraceID)
			}
			if strings.Contains(tracestate, "p:") {
				ddParentID := strings.Split(strings.Split(tracestate, "p:")[1], ";")[0]
				if strings.Contains(ddParentID, " ") {
					ddParentID = strings.Split(ddParentID, " ")[0]
				}
				r.Header.Set("x-datadog-parent-id", ddParentID)
			}
			if strings.Contains(tracestate, "s:") {
				ddSampling := strings.Split(strings.Split(tracestate, "s:")[1], ";")[0]
				if strings.Contains(ddSampling, " ") {
					ddSampling = strings.Split(ddSampling, " ")[0]
				}
				r.Header.Set("x-datadog-sampling-priority", ddSampling)
			}
		}
	}

	// Fallback to direct Datadog headers if available
	if datadogTraceID := attrs["x-datadog-trace-id"]; datadogTraceID != "" && r.Header.Get("x-datadog-trace-id") == "" {
		r.Header.Set("x-datadog-trace-id", datadogTraceID)
	}
	if datadogParentID := attrs["x-datadog-parent-id"]; datadogParentID != "" && r.Header.Get("x-datadog-parent-id") == "" {
		r.Header.Set("x-datadog-parent-id", datadogParentID)
	}
	if datadogSamplingPriority := attrs["x-datadog-sampling-priority"]; datadogSamplingPriority != "" && r.Header.Get("x-datadog-sampling-priority") == "" {
		r.Header.Set("x-datadog-sampling-priority", datadogSamplingPriority)
	} else if r.Header.Get("x-datadog-sampling-priority") == "" {
		r.Header.Set("x-datadog-sampling-priority", "1") // Default to sampled
	}
	if datadogOrigin := attrs["x-datadog-origin"]; datadogOrigin != "" {
		r.Header.Set("x-datadog-origin", datadogOrigin)
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
	targetURL := "http://localhost:8081" + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	fmt.Printf("CLOUDRUN_PROXY: Forwarding request to %s\n", targetURL)
	fmt.Printf("CLOUDRUN_PROXY: Request headers being forwarded: %+v\n", r.Header)
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
	req.Host = "localhost:8081"

	// Make request to target service
	fmt.Printf("CLOUDRUN_PROXY: Making request to target service\n")
	resp, err := p.client.Do(req)
	if err != nil {
		fmt.Printf("CLOUDRUN_PROXY: Error forwarding request: %v\n", err)
		log.Errorf("Cloud Run proxy: error forwarding request: %v", err)
		if errors.Is(err, context.DeadlineExceeded) {
			w.WriteHeader(http.StatusGatewayTimeout)
		} else {
			w.WriteHeader(http.StatusBadGateway)
		}
		return
	}
	defer resp.Body.Close()

	fmt.Printf("CLOUDRUN_PROXY: Received response with status %d\n", resp.StatusCode)
	fmt.Printf("CLOUDRUN_PROXY: Response headers: %+v\n", resp.Header)

	// Copy response headers and body efficiently
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		fmt.Printf("CLOUDRUN_PROXY: Error copying response body: %v\n", err)
		log.Errorf("Cloud Run proxy: error copying response body: %v", err)
	}

	fmt.Printf("CLOUDRUN_PROXY: Request forwarding completed successfully\n")
	log.Debugf("Cloud Run proxy: forwarded request completed with status %d", resp.StatusCode)
}
