// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package healthplatformimpl

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
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// intakeEndpointPrefix is the URL prefix for the Datadog intake endpoint
	intakeEndpointPrefix = "https://event-platform-intake."

	// intakeEndpointPath is the API path for agent health reports
	intakeEndpointPath = "/api/v2/agenthealth"

	// defaultForwarderInterval is the default interval between health report submissions
	defaultForwarderInterval = 15 * time.Minute

	// httpTimeout is the timeout for HTTP requests
	httpTimeout = 30 * time.Second

	// eventType is the event type for health reports
	eventType = "agent-health-issues"
)

// issueProvider defines the interface for retrieving health issues
type issueProvider interface {
	GetAllIssues() (int, map[string]*healthplatform.Issue)
}

// forwarder handles periodic sending of health reports to the Datadog intake
type forwarder struct {
	cfg        pkgconfigmodel.Reader
	intakeURL  string
	interval   time.Duration
	hostname   string
	provider   issueProvider
	httpClient *http.Client
	log        log.Component

	stopCh chan struct{}
	doneCh chan struct{}
}

// newForwarder creates a new forwarder instance
func newForwarder(
	cfg pkgconfigmodel.Reader,
	provider issueProvider,
	logger log.Component,
	hostname string,
) *forwarder {
	interval := cfg.GetDuration("health_platform.forwarder.interval")
	if interval <= 0 {
		interval = defaultForwarderInterval
	}

	return &forwarder{
		cfg:        cfg,
		intakeURL:  buildIntakeURL(cfg),
		interval:   interval,
		hostname:   hostname,
		provider:   provider,
		httpClient: buildHTTPClient(cfg),
		log:        logger,
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
	}
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
func (f *forwarder) Start() {
	f.log.Info(fmt.Sprintf("Starting health platform forwarder with %v interval to %s", f.interval, f.intakeURL))

	go f.run()
}

// Stop stops the forwarder and waits for graceful shutdown
func (f *forwarder) Stop() {
	f.log.Info("Stopping health platform forwarder")
	close(f.stopCh)
	<-f.doneCh
}

// run is the main loop that sends health reports periodically
func (f *forwarder) run() {
	defer close(f.doneCh)

	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			f.sendHealthReport()
		case <-f.stopCh:
			return
		}
	}
}

// sendHealthReport collects issues and sends them to the intake endpoint
func (f *forwarder) sendHealthReport() {
	count, issues := f.provider.GetAllIssues()

	if count == 0 {
		f.log.Info("No health issues to report")
		return
	}

	report := f.buildReport(issues)

	if err := f.send(report); err != nil {
		f.log.Warn(fmt.Sprintf("Failed to send health report: %v", err))
		return
	}

	f.log.Info(fmt.Sprintf("Successfully sent health report with %d issues", count))
}

// buildReport creates a HealthReport from the current issues
func (f *forwarder) buildReport(issues map[string]*healthplatform.Issue) *healthplatform.HealthReport {
	return &healthplatform.HealthReport{
		EventType: eventType,
		EmittedAt: time.Now().UTC().Format(time.RFC3339),
		Host: &healthplatform.HostInfo{
			Hostname:     f.hostname,
			AgentVersion: version.AgentVersion,
		},
		Issues: issues,
	}
}

// send marshals and sends the report to the intake endpoint
func (f *forwarder) send(report *healthplatform.HealthReport) error {
	// Fetch API key once and check if configured
	apiKey := f.cfg.GetString("api_key")
	if apiKey == "" {
		return errors.New("API key not configured")
	}

	payload, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.intakeURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", apiKey)
	req.Header.Set("DD-Agent-Version", version.AgentVersion)
	req.Header.Set("User-Agent", "datadog-agent/"+version.AgentVersion)

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
