// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package healthplatformimpl provides the implementation for the health platform component.
package healthplatformimpl

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	healthplatform "github.com/DataDog/datadog-agent/comp/core/health-platform/def"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// DefaultTickerInterval is the default interval for health check ticker
const DefaultTickerInterval = 5 * time.Minute

// Dependencies defines the dependencies for the health platform component
type Dependencies struct {
	Config    config.Component
	Hostname  hostname.Component
	Forwarder defaultforwarder.Component `optional:"true"`
}

// component implements the health platform component
type component struct {
	cfg       config.Component
	hostname  hostname.Component
	forwarder defaultforwarder.Component

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
		cfg:       deps.Config,
		hostname:  deps.Hostname,
		forwarder: deps.Forwarder,
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
	log.Info("Running health checks on all sub-components")

	allIssues := c.collectHealthCheckResults(ctx)

	// Emit results directly
	if len(allIssues) == 0 {
		log.Info("All health checks passed - no issues found")
	} else {
		log.Infof("Health checks completed - found %d issues", len(allIssues))

		for _, issue := range allIssues {
			log.Infof("Issue: %s - %s (Severity: %s, Location: %s)",
				issue.ID, issue.Description, issue.Severity, issue.Location)
		}
	}

	// Format and return the report
	report := formatHealthReport(allIssues, c.hostInfo)
	return &report, nil
}

// FlushIssues flushes the current issues to the backend
func (c *component) FlushIssues() error {
	return c.SubmitReport(context.Background())
}

// RegisterSubComponent registers a health checker sub-component
func (c *component) RegisterSubComponent(sub healthplatform.SubComponent) error {
	if sub == nil {
		return fmt.Errorf("sub-component cannot be nil")
	}

	c.subComponentsMu.Lock()
	defer c.subComponentsMu.Unlock()

	c.subComponents = append(c.subComponents, sub)
	log.Debugf("Registered sub-component: %T", sub)
	return nil
}

// SubmitReport immediately submits the current issues to the backend
func (c *component) SubmitReport(ctx context.Context) error {
	log.Info("Submitting health platform report")

	allIssues := c.collectHealthCheckResults(ctx)

	if len(allIssues) == 0 {
		log.Info("No health issues found")
	} else {
		log.Infof("Found %d health issues", len(allIssues))

		// Format the report
		report := formatHealthReport(allIssues, c.hostInfo)

		// Log the formatted report (for now, later this will be sent to intake)
		log.Infof("Formatted health report: %+v", report)

		for _, issue := range allIssues {
			log.Infof("Issue: %s - %s (Severity: %s, Location: %s)",
				issue.ID, issue.Description, issue.Severity, issue.Location)
		}
	}

	return nil
}

// startTicker starts the periodic health check ticker
func (c *component) startTicker() {
	defer close(c.done)

	ticker := time.NewTicker(DefaultTickerInterval)
	defer ticker.Stop()

	log.Infof("Health platform ticker started with interval: %v", DefaultTickerInterval)

	for {
		select {
		case <-c.ctx.Done():
			log.Info("Health platform ticker stopped")
			return
		case <-ticker.C:
			log.Debug("Running periodic health checks")
			if _, err := c.Run(c.ctx); err != nil {
				log.Warnf("Failed to run periodic health checks: %v", err)
			}
		}
	}
}

// collectHealthCheckResults collects issues from all registered sub-components
func (c *component) collectHealthCheckResults(ctx context.Context) []healthplatform.Issue {
	c.subComponentsMu.RLock()
	defer c.subComponentsMu.RUnlock()

	var allIssues []healthplatform.Issue

	for _, sub := range c.subComponents {
		issues, err := sub.CheckHealth(ctx)
		if err != nil {
			log.Debugf("Sub-component %T failed health check: %v", sub, err)
			continue
		}
		allIssues = append(allIssues, issues...)
	}

	return allIssues
}
