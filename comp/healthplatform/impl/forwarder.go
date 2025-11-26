// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package healthplatformimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
	pkgconfigutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// intakeEndpointPrefix is the prefix for the Datadog intake endpoint for agent health
	intakeEndpointPrefix = "https://event-platform-intake."
	// intakeEndpointPath is the API path for agent health
	intakeEndpointPath = "/api/v2/agenthealth"
	// defaultForwarderInterval is the default interval for sending health reports
	defaultForwarderInterval = 15 * time.Minute
)

// forwarder handles periodic sending of health reports to Datadog intake
type forwarder struct {
	// Dependencies
	comp   *healthPlatformImpl
	config config.Component

	// HTTP client for sending requests
	httpClient *http.Client

	// Pre-configured headers (including API key)
	headerLock sync.RWMutex
	headers    http.Header

	// Control channels
	stopCh chan struct{}
	ctx    context.Context
	cancel context.CancelFunc

	// Cached metadata
	hostname  string
	intakeURL string // Computed intake URL based on site configuration
}

// newForwarder creates a new forwarder instance
func newForwarder(comp *healthPlatformImpl, hostname string, apiKey string) *forwarder {
	ctx, cancel := context.WithCancel(context.Background())

	// Create HTTP client with agent-standard transport configuration
	transport := httputils.CreateHTTPTransport(comp.config)
	httpClient := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	// Pre-configure headers including API key (stored once, used for all requests)
	// Use canonical header keys to ensure proper lookup
	headers := http.Header{
		http.CanonicalHeaderKey("Content-Type"):     []string{"application/json"},
		http.CanonicalHeaderKey("DD-API-KEY"):       []string{apiKey},
		http.CanonicalHeaderKey("DD-Agent-Version"): []string{version.AgentVersion},
		http.CanonicalHeaderKey("User-Agent"):       []string{fmt.Sprintf("datadog-agent/%s", version.AgentVersion)},
	}

	return &forwarder{
		comp:       comp,
		config:     comp.config,
		httpClient: httpClient,
		headers:    headers,
		headerLock: sync.RWMutex{},
		stopCh:     make(chan struct{}),
		ctx:        ctx,
		cancel:     cancel,
		hostname:   hostname,
		intakeURL:  computeIntakeURL(comp.config),
	}
}

// computeIntakeURL builds the intake URL based on the configured site
func computeIntakeURL(cfg config.Component) string {
	// Use the standard GetMainEndpoint function to build the URL based on site config
	baseURL := pkgconfigutils.GetMainEndpoint(cfg, intakeEndpointPrefix, "dd_url")
	return baseURL + intakeEndpointPath
}

// start begins the periodic forwarding of health reports
func (f *forwarder) start() {
	// Get the interval from config with default fallback
	interval := f.config.GetDuration("health_platform.forwarder.interval_minutes") * time.Minute
	if interval <= 0 {
		interval = defaultForwarderInterval
	}

	f.comp.log.Info(fmt.Sprintf("Starting health platform forwarder with %v interval", interval))

	go func() {
		// Periodic sending
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				f.sendHealthReport()
			case <-f.stopCh:
				return
			}
		}
	}()
}

// stop stops the forwarder
func (f *forwarder) stop() {
	f.comp.log.Info("Stopping health platform forwarder")
	close(f.stopCh)
	f.cancel()
}

// sendHealthReport collects all issues and sends them to the intake endpoint
func (f *forwarder) sendHealthReport() {
	// Collect all issues
	count, issuesMap := f.comp.GetAllIssues()

	// Only send if there are issues
	if count == 0 {
		f.comp.log.Info("No health issues to report")
		return
	}

	// Build the health report
	report := &healthplatform.HealthReport{
		SchemaVersion: "1.0",
		EventType:     "agent-health-issues",
		EmittedAt:     time.Now().Format(time.RFC3339),
		Host: healthplatform.HostInfo{
			Hostname:     f.hostname,
			AgentVersion: version.AgentVersion,
		},
		Issues: issuesMap,
	}

	// Marshal the report to JSON
	payload, err := json.Marshal(report)
	if err != nil {
		f.comp.log.Warn("Failed to marshal health report: " + err.Error())
		return
	}

	// Create the HTTP request
	req, err := http.NewRequestWithContext(f.ctx, "POST", f.intakeURL, bytes.NewBuffer(payload))
	if err != nil {
		f.comp.log.Warn("Failed to create request: " + err.Error())
		return
	}

	// Apply pre-configured headers (including API key)
	f.applyHeaders(req)

	// Send the request
	resp, err := f.httpClient.Do(req)
	if err != nil {
		f.comp.log.Warn("Failed to send request: " + err.Error())
		return
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		f.comp.log.Warn(fmt.Sprintf("Received non-success status code: %d", resp.StatusCode))
		return
	}

	f.comp.log.Info(fmt.Sprintf("Successfully sent health report with %d issues", count))
}

// applyHeaders applies the pre-configured headers to a request
func (f *forwarder) applyHeaders(req *http.Request) {
	f.headerLock.RLock()
	defer f.headerLock.RUnlock()

	for key, values := range f.headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
}
