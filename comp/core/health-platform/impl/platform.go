// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package healthplatformimpl provides the implementation for the health platform component.
package healthplatformimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	healthplatform "github.com/DataDog/datadog-agent/comp/core/health-platform/def"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// DefaultTickerInterval is the default interval for health check ticker
const DefaultTickerInterval = 5 * time.Minute

// healthClient handles HTTP communication with the health platform backend
type healthClient struct {
	apiKey   string
	site     string
	endpoint string
	client   *http.Client
}

// newHealthClient creates a new health client with configuration
func newHealthClient(cfg config.Component) *healthClient {
	apiKey := utils.SanitizeAPIKey(cfg.GetString("api_key"))
	site := cfg.GetString("site")
	if site == "" {
		site = "app.datadoghq.com" // default site
	}
	endpoint := fmt.Sprintf("https://%s/api/v2/agent-recommendation-health", site)

	return &healthClient{
		apiKey:   apiKey,
		site:     site,
		endpoint: endpoint,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Dependencies defines the dependencies for the health platform component
type Dependencies struct {
	Config              config.Component
	Hostname            hostname.Component
	Forwarder           defaultforwarder.Component    `optional:"true"`
	PrivateActionRunner privateactionrunner.Component `optional:"true"`
}

// component implements the health platform component
type component struct {
	cfg                 config.Component
	hostname            hostname.Component
	forwarder           defaultforwarder.Component
	privateActionRunner privateactionrunner.Component

	// HTTP client for backend communication
	healthClient *healthClient

	// sub-components for health checks
	subComponents   []healthplatform.SubComponent
	subComponentsMu sync.RWMutex

	// lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	// host information for reports
	hostInfo healthplatform.HostInfo
}

// NewComponent creates a new health platform component
func NewComponent(deps Dependencies) healthplatform.Component {
	return &component{
		cfg:                 deps.Config,
		hostname:            deps.Hostname,
		forwarder:           deps.Forwarder,
		privateActionRunner: deps.PrivateActionRunner,
		healthClient:        newHealthClient(deps.Config),
		hostInfo: healthplatform.HostInfo{
			AgentVersion: version.AgentVersion,
			ParIDs:       []string{}, // Will be populated later
		},
	}
}

// Start begins the periodic reporting of issues
func (c *component) Start(ctx context.Context) error {
	// Get hostname and populate host info
	if c.hostname != nil {
		if hostname, err := c.hostname.Get(ctx); err == nil {
			c.hostInfo.Hostname = hostname
		} else {
			log.Warnf("Failed to get hostname: %v", err)
			c.hostInfo.Hostname = "unknown"
		}
	} else {
		c.hostInfo.Hostname = "unknown"
	}

	// Populate ParIDs from private action runner if available
	if c.privateActionRunner != nil {
		if runnerID := c.privateActionRunner.GetRunnerID(); runnerID != "" {
			c.hostInfo.ParIDs = []string{runnerID}
			log.Infof("Health platform initialized with PAR ID: %s", runnerID)
		} else {
			log.Info("Private action runner is available but no runner ID configured")
		}
	} else {
		log.Info("No private action runner available for health platform")
	}

	// Start the ticker for periodic health checks
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.done = make(chan struct{})

	go c.startTicker()

	return nil
}

// Stop stops the periodic reporting
func (c *component) Stop() error {
	if c.cancel != nil {
		c.cancel()
		<-c.done
		c.cancel = nil
		c.done = nil
	}

	return nil
}

// Run runs the health checks and reports the issues
func (c *component) Run(ctx context.Context) (*healthplatform.HealthReport, error) {
	var allIssues []healthplatform.Issue
	var err error
	if allIssues, err = c.collectHealthCheckResults(ctx); err != nil {
		return nil, err
	}

	// Emit results directly
	if len(allIssues) == 0 {
		log.Info("All health checks passed - no issues found")
		return nil, nil
	}

	log.Infof("Health checks completed - found %d issues", len(allIssues))

	// Format and return the report in JSON:API format
	jsonAPIResponse := formatJSONAPIResponse(allIssues, c.hostInfo)

	// Extract the health report from the JSON:API response for backward compatibility
	healthReport := *jsonAPIResponse.Data
	return &healthReport, nil
}

// RegisterSubComponent registers a health checker sub-component
func (c *component) RegisterSubComponent(sub healthplatform.SubComponent) error {
	if sub == nil {
		return fmt.Errorf("sub-component cannot be nil")
	}

	c.subComponentsMu.Lock()
	defer c.subComponentsMu.Unlock()

	c.subComponents = append(c.subComponents, sub)
	return nil
}

// sendReport sends a health report to the backend in JSON:API format
func (hc *healthClient) sendReport(ctx context.Context, report *healthplatform.HealthReport) error {
	if report == nil {
		return fmt.Errorf("health report cannot be nil")
	}

	if hc.apiKey == "" {
		return fmt.Errorf("API key not configured")
	}

	// Create JSON:API response with proper structure
	jsonAPIResponse := map[string]interface{}{
		"data": map[string]interface{}{
			"type":       "agent_health_payload",
			"attributes": report, // This wraps your HealthReport in the attributes
		},
		"meta": map[string]interface{}{
			"schema_version": report.SchemaVersion,
			"event_type":     report.EventType,
			"emitted_at":     report.EmittedAt,
		},
	}

	// Marshal the JSON:API response to JSON
	jsonData, err := json.Marshal(jsonAPIResponse)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON:API response: %w", err)
	}

	// Create and send the HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", hc.endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers for JSON:API format
	req.Header.Set("dd-api-key", hc.apiKey)
	req.Header.Set("content-type", "application/vnd.api+json")
	req.Header.Set("Accept", "application/vnd.api+json")

	resp, err := hc.client.Do(req)
	if err != nil {
		log.Errorf("Failed to send health report to backend: %v", err)
		return fmt.Errorf("failed to send health report to backend: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Errorf("Backend returned error status: %d", resp.StatusCode)
		return fmt.Errorf("failed to send health report to backend: backend returned error status: %d", resp.StatusCode)
	}

	log.Infof("Successfully emitted health report to backend. Issues found: %d", len(report.Issues))
	return nil
}

// EmitToBackend emits the current health report to a custom backend service
func (c *component) EmitToBackend(ctx context.Context, report *healthplatform.HealthReport) error {
	return c.healthClient.sendReport(ctx, report)
}

// startTicker starts the periodic health check ticker
func (c *component) startTicker() {
	defer close(c.done)

	ticker := time.NewTicker(DefaultTickerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			log.Info("Health platform ticker stopped")
			return
		case <-ticker.C:
			if report, err := c.Run(c.ctx); err != nil {
				log.Warnf("Failed to run periodic health checks: %v", err)
			} else if report != nil && c.cfg.GetBool("health_platform.enabled") {
				if err := c.EmitToBackend(c.ctx, report); err != nil {
					log.Warnf("Failed to emit health report to backend: %v", err)
				}
			}
		}
	}
}

// collectHealthCheckResults collects issues from all registered sub-components
func (c *component) collectHealthCheckResults(ctx context.Context) ([]healthplatform.Issue, error) {
	c.subComponentsMu.RLock()
	defer c.subComponentsMu.RUnlock()

	var allIssues []healthplatform.Issue
	var errs []error

	for _, sub := range c.subComponents {
		issues, err := sub.CheckHealth(ctx)
		if err != nil {
			log.Debugf("Sub-component %T failed health check: %v", sub, err)
			errs = append(errs, err)
			continue
		}
		allIssues = append(allIssues, issues...)
	}

	return allIssues, errors.Join(errs...)
}
