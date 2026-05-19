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
	"sync"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
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
	intakeEndpointPrefix = "https://agenthealth-intake."

	// intakeEndpointPath is the API path for agent health reports
	intakeEndpointPath = "/api/v2/agenthealth"

	// defaultForwarderInterval is the default interval between health report submissions
	defaultForwarderInterval = 15 * time.Minute

	// httpTimeout is the timeout for HTTP requests
	httpTimeout = 30 * time.Second

	// eventType is the event type for health reports
	eventType = "agent-health-issues"
)

// forwarder handles periodic sending of health reports to the Datadog intake
type forwarder struct {
	cfg         pkgconfigmodel.Reader
	intakeURL   string
	interval    time.Duration
	hostname    string
	agentFlavor string
	providerMu  sync.RWMutex
	provider    forwarderdef.IssueProvider
	httpClient  *http.Client
	log         log.Component

	stopCh chan struct{}
	doneCh chan struct{}
}

// Requires defines the dependencies for the forwarder.
type Requires struct {
	Log       log.Component
	Config    config.Component
	Hostname  hostnameinterface.Component
	Lifecycle compdef.Lifecycle
}

// New creates a new forwarder instance and registers its lifecycle hooks.
func New(reqs Requires) forwarderdef.Component {
	if !reqs.Config.GetBool("health_platform.enabled") {
		return &forwarder{log: reqs.Log}
	}

	hostname, err := reqs.Hostname.Get(context.Background())
	if err != nil {
		reqs.Log.Warn("Health platform forwarder: failed to get hostname, will use empty string: " + err.Error())
		hostname = ""
	}

	interval := reqs.Config.GetDuration("health_platform.forwarder.interval")
	if interval <= 0 {
		interval = defaultForwarderInterval
	}

	f := &forwarder{
		cfg:         reqs.Config,
		intakeURL:   buildIntakeURL(reqs.Config),
		interval:    interval,
		hostname:    hostname,
		agentFlavor: flavor.GetFlavor(),
		httpClient:  buildHTTPClient(reqs.Config),
		log:         reqs.Log,
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: f.start,
		OnStop:  f.stop,
	})

	return f
}

func (f *forwarder) start(_ context.Context) error {
	f.log.Info(fmt.Sprintf("Starting health platform forwarder with %v interval to %s", f.interval, f.intakeURL))
	go f.run()
	return nil
}

func (f *forwarder) stop(_ context.Context) error {
	f.log.Info("Stopping health platform forwarder")
	close(f.stopCh)
	<-f.doneCh
	return nil
}

// SetProvider wires the issue provider. Safe to call concurrently with sendHealthReport.
func (f *forwarder) SetProvider(provider forwarderdef.IssueProvider) {
	f.providerMu.Lock()
	defer f.providerMu.Unlock()
	f.provider = provider
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
	f.providerMu.RLock()
	provider := f.provider
	f.providerMu.RUnlock()

	if provider == nil {
		f.log.Warn("Health platform forwarder has no provider set, skipping report")
		return
	}
	count, issues := provider.GetAllIssues()

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
		Service:   f.agentFlavor,
		Host: &healthplatform.HostInfo{
			Hostname:     f.hostname,
			AgentVersion: pointer.Ptr(version.AgentVersion),
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
