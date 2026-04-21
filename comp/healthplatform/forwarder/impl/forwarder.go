// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package forwarderimpl implements the health platform forwarder component.
package forwarderimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	forwarderdef "github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// intakeEndpointPrefix is the URL prefix for the Datadog intake endpoint
	intakeEndpointPrefix = "https://event-platform-intake."

	// intakeEndpointPath is the API path for agent health reports
	intakeEndpointPath = "/api/v2/agenthealth"

	// defaultReporterInterval is the default interval between health report submissions
	defaultReporterInterval = 15 * time.Minute

	// httpTimeout is the timeout for HTTP requests
	httpTimeout = 30 * time.Second

	// eventType is the event type for health reports
	eventType = "agent-health-issues"
)

// forwarder handles periodic sending of health reports to the Datadog intake
type forwarder struct {
	cfg        pkgconfigmodel.Reader
	intakeURL  string
	interval   time.Duration
	hostname   string
	provider   forwarderdef.IssueProvider
	httpClient *http.Client
	log        log.Component

	stopCh chan struct{}
	doneCh chan struct{}
}

// New creates a new forwarder instance with an explicit hostname.
// Call SetProvider before Start() to wire the issue provider.
func New(
	logger log.Component,
	cfg pkgconfigmodel.Reader,
	hostname string,
) forwarderdef.Component {
	interval := cfg.GetDuration("health_platform.forwarder.interval")
	if interval <= 0 {
		interval = defaultReporterInterval
	}

	return &forwarder{
		cfg:        cfg,
		intakeURL:  buildIntakeURL(cfg),
		interval:   interval,
		hostname:   hostname,
		httpClient: buildHTTPClient(cfg),
		log:        logger,
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
	}
}

// SetProvider wires the issue provider. Must be called before Start().
func (r *forwarder) SetProvider(provider forwarderdef.IssueProvider) {
	r.provider = provider
}

// buildIntakeURL constructs the intake URL based on site configuration
func buildIntakeURL(cfg pkgconfigmodel.Reader) string {
	baseURL := configutils.GetMainEndpoint(cfg, intakeEndpointPrefix, "dd_url")
	return baseURL + intakeEndpointPath
}

// buildHTTPClient creates an HTTP client with agent-standard transport configuration
func buildHTTPClient(cfg pkgconfigmodel.Reader) *http.Client {
	return &http.Client{
		Timeout:   httpTimeout,
		Transport: httputils.CreateHTTPTransport(cfg),
	}
}

// Start begins the periodic forwarding of health reports
func (r *forwarder) Start() {
	r.log.Info(fmt.Sprintf("Starting health platform forwarder with %v interval to %s", r.interval, r.intakeURL))

	go r.run()
}

// Stop stops the forwarder and waits for graceful shutdown
func (r *forwarder) Stop() {
	r.log.Info("Stopping health platform forwarder")
	close(r.stopCh)
	<-r.doneCh
}

// run is the main loop that sends health reports periodically
func (r *forwarder) run() {
	defer close(r.doneCh)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.sendHealthReport()
		case <-r.stopCh:
			return
		}
	}
}

// sendHealthReport collects issues and sends them to the intake endpoint
func (r *forwarder) sendHealthReport() {
	if r.provider == nil {
		r.log.Warn("Health platform forwarder has no provider set, skipping report")
		return
	}
	count, issues := r.provider.GetAllIssues()

	if count == 0 {
		r.log.Info("No health issues to report")
		return
	}

	report := r.buildReport(issues)

	if err := r.send(report); err != nil {
		r.log.Warn(fmt.Sprintf("Failed to send health report: %v", err))
		return
	}

	r.log.Info(fmt.Sprintf("Successfully sent health report with %d issues", count))
}

// safeGetFlavor returns the current agent flavor, falling back to the default
// if flavor.GetFlavor() panics (e.g. called before main init).
func safeGetFlavor() (f string) {
	defer func() {
		if r := recover(); r != nil {
			f = flavor.DefaultAgent
		}
	}()
	return flavor.GetFlavor()
}

// buildReport creates a HealthReport from the current issues
func (r *forwarder) buildReport(issues map[string]*healthplatform.Issue) *healthplatform.HealthReport {
	return &healthplatform.HealthReport{
		EventType: eventType,
		EmittedAt: time.Now().UTC().Format(time.RFC3339),
		Service:   safeGetFlavor(),
		Host: &healthplatform.HostInfo{
			Hostname:     r.hostname,
			AgentVersion: pointer.Ptr(version.AgentVersion),
		},
		Issues: issues,
	}
}

// send marshals and sends the report to the intake endpoint
func (r *forwarder) send(report *healthplatform.HealthReport) error {
	// Fetch API key once and check if configured
	apiKey := r.cfg.GetString("api_key")
	if apiKey == "" {
		return errors.New("API key not configured")
	}

	payload, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.intakeURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", apiKey)
	req.Header.Set("DD-Agent-Version", version.AgentVersion)
	req.Header.Set("User-Agent", "datadog-agent/"+version.AgentVersion)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
