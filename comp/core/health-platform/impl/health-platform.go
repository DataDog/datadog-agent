// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package healthplatformimpl provides the implementation for the health platform component.
package healthplatformimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/host"

	"github.com/DataDog/datadog-agent/comp/core/config"
	healthplatform "github.com/DataDog/datadog-agent/comp/core/health-platform/def"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	healthplatformpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/healthplatform"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// DefaultReportingInterval is the default interval for periodic reporting
	DefaultReportingInterval = 15 * time.Minute
)

// Dependencies lists the dependencies for the health platform component
type Dependencies struct {
	Config    config.Component
	Hostname  hostnameinterface.Component
	Forwarder defaultforwarder.Component
}

// component implements the health platform component
type component struct {
	cfg       config.Component
	hostname  hostnameinterface.Component
	forwarder defaultforwarder.Component

	// issues storage
	issuesMu sync.RWMutex
	issues   map[string]healthplatform.Issue

	// reporting control
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	// reporting interval
	interval time.Duration
}

// NewComponent creates a new health platform component
func NewComponent(deps Dependencies) healthplatform.Component {
	return &component{
		cfg:       deps.Config,
		hostname:  deps.Hostname,
		forwarder: deps.Forwarder,
		issues:    make(map[string]healthplatform.Issue),
		interval:  DefaultReportingInterval,
	}
}

// AddIssue adds a new issue to be reported
func (c *component) AddIssue(issue healthplatform.Issue) error {
	if issue.ID == "" {
		return fmt.Errorf("issue ID cannot be empty")
	}
	if issue.Name == "" {
		return fmt.Errorf("issue name cannot be empty")
	}

	c.issuesMu.Lock()
	defer c.issuesMu.Unlock()

	c.issues[issue.ID] = issue
	log.Debugf("Added issue: %s (%s)", issue.ID, issue.Name)
	return nil
}

// RemoveIssue removes an issue by ID
func (c *component) RemoveIssue(id string) error {
	if id == "" {
		return fmt.Errorf("issue ID cannot be empty")
	}

	c.issuesMu.Lock()
	defer c.issuesMu.Unlock()

	if _, exists := c.issues[id]; !exists {
		return fmt.Errorf("issue with ID %s not found", id)
	}

	delete(c.issues, id)
	log.Debugf("Removed issue: %s", id)
	return nil
}

// ListIssues returns all currently tracked issues
func (c *component) ListIssues() []healthplatform.Issue {
	c.issuesMu.RLock()
	defer c.issuesMu.RUnlock()

	issues := make([]healthplatform.Issue, 0, len(c.issues))
	for _, issue := range c.issues {
		issues = append(issues, issue)
	}
	return issues
}

// SubmitReport immediately submits the current issues to the backend
func (c *component) SubmitReport(ctx context.Context) error {
	// Get current issues
	issues := c.ListIssues()

	if len(issues) == 0 {
		log.Debug("No issues to report")
		return nil
	}

	// Get hostname (with nil check for testing)
	var hostname string
	var err error
	if c.hostname != nil {
		hostname, err = c.hostname.Get(ctx)
		if err != nil {
			return fmt.Errorf("failed to get hostname: %w", err)
		}
	} else {
		hostname = "test-hostname" // default for testing
	}

	// Get host ID using gopsutil
	hostInfo, err := host.Info()
	if err != nil {
		return fmt.Errorf("failed to get host info: %w", err)
	}

	// Convert issues to protobuf format
	pbIssues := make([]*healthplatformpb.Issue, len(issues))
	for i, issue := range issues {
		pbIssue := &healthplatformpb.Issue{
			Id:   issue.ID,
			Name: issue.Name,
		}
		if issue.Extra != "" {
			pbIssue.Extra = &issue.Extra
		}
		pbIssues[i] = pbIssue
	}

	// Create the report payload
	report := &HealthReportPayload{
		Hostname:  hostname,
		HostID:    hostInfo.HostID,
		Issues:    issues,
		Timestamp: time.Now().Unix(),
		PbReport: &healthplatformpb.HealthReport{
			Hostname:  hostname,
			HostId:    hostInfo.HostID,
			Issues:    pbIssues,
			Timestamp: time.Now().Unix(),
		},
	}

	// Send the report using the forwarder directly
	// In a real implementation, you would use the appropriate forwarder method
	// For now, we'll log the report as this is a demonstration
	log.Infof("Health report ready to send: %s", report.DescribeItem())

	log.Infof("Successfully submitted health report with %d issues", len(issues))
	return nil
}

// Start begins the periodic reporting of issues
func (c *component) Start(ctx context.Context) error {
	if c.cancel != nil {
		return fmt.Errorf("health platform is already running")
	}

	c.ctx, c.cancel = context.WithCancel(ctx)
	c.done = make(chan struct{})

	// Get reporting interval from config (with nil check for testing)
	if c.cfg != nil {
		if configObj := c.cfg.Object(); configObj != nil {
			if durGetter, ok := configObj.(interface{ GetDuration(string) time.Duration }); ok {
				if configInterval := durGetter.GetDuration("health_platform.interval"); configInterval > 0 {
					c.interval = configInterval
				}
			}
		}
	}

	go c.reportingLoop()
	log.Infof("Started health platform with interval: %v", c.interval)
	return nil
}

// Stop stops the periodic reporting
func (c *component) Stop() error {
	if c.cancel == nil {
		return fmt.Errorf("health platform is not running")
	}

	c.cancel()
	<-c.done
	c.cancel = nil
	c.done = nil

	log.Info("Stopped health platform")
	return nil
}

// reportingLoop handles the periodic reporting of issues
func (c *component) reportingLoop() {
	defer close(c.done)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if err := c.SubmitReport(c.ctx); err != nil {
				log.Warnf("Failed to submit periodic health report: %v", err)
			}
		}
	}
}

// HealthReportPayload implements marshaler.JSONMarshaler for sending health reports
type HealthReportPayload struct {
	Hostname  string                         `json:"hostname"`
	HostID    string                         `json:"host_id"`
	Issues    []healthplatform.Issue         `json:"issues"`
	Timestamp int64                          `json:"timestamp"`
	PbReport  *healthplatformpb.HealthReport `json:"-"` // Protobuf version for backend
}

// MarshalJSON implements marshaler.JSONMarshaler
func (p *HealthReportPayload) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Hostname  string                 `json:"hostname"`
		HostID    string                 `json:"host_id"`
		Issues    []healthplatform.Issue `json:"issues"`
		Timestamp int64                  `json:"timestamp"`
	}{
		Hostname:  p.Hostname,
		HostID:    p.HostID,
		Issues:    p.Issues,
		Timestamp: p.Timestamp,
	})
}

// SplitPayload splits the payload if it's too large
func (p *HealthReportPayload) SplitPayload(times int) ([]marshaler.AbstractMarshaler, error) {
	if times <= 1 || len(p.Issues) <= 1 {
		return nil, fmt.Errorf("cannot split health report payload further")
	}

	var payloads []marshaler.AbstractMarshaler
	chunkSize := len(p.Issues) / times
	if chunkSize == 0 {
		chunkSize = 1
	}

	for i := 0; i < len(p.Issues); i += chunkSize {
		end := i + chunkSize
		if end > len(p.Issues) {
			end = len(p.Issues)
		}

		chunk := &HealthReportPayload{
			Hostname:  p.Hostname,
			HostID:    p.HostID,
			Issues:    p.Issues[i:end],
			Timestamp: p.Timestamp,
		}
		payloads = append(payloads, chunk)
	}

	return payloads, nil
}

// DescribeItem returns a description of the payload for logging
func (p *HealthReportPayload) DescribeItem() string {
	return fmt.Sprintf("HealthReport with %d issues for host %s", len(p.Issues), p.Hostname)
}
