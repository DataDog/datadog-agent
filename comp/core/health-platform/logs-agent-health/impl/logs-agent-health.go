// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package logsagenthealthimpl provides the implementation for the logs agent health checker sub-component.
package logsagenthealthimpl

import (
	"context"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	healthplatform "github.com/DataDog/datadog-agent/comp/core/health-platform/def"
	logsagenthealth "github.com/DataDog/datadog-agent/comp/core/health-platform/logs-agent-health/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Dependencies lists the dependencies for the logs agent health checker
type Dependencies struct {
	Config config.Component
}

// component implements the logs agent health checker sub-component
type component struct {
	cfg config.Component

	// sub-checks storage
	subChecksMu sync.RWMutex
	subChecks   []logsagenthealth.SubCheck
}

// NewComponent creates a new logs agent health checker component
func NewComponent(deps Dependencies) logsagenthealth.Component {
	log.Infof("Creating new logs agent health checker component")

	if deps.Config == nil {
		log.Warnf("Config dependency is nil, logs agent health checker may not function properly")
	}

	comp := &component{
		cfg:       deps.Config,
		subChecks: make([]logsagenthealth.SubCheck, 0),
	}

	log.Infof("Component initialized with config: %v", deps.Config != nil)

	// Register default sub-checks
	comp.registerDefaultSubChecks()

	log.Infof("Logs agent health checker component created successfully")
	return comp
}

// registerDefaultSubChecks registers the default set of health checks
func (c *component) registerDefaultSubChecks() {
	log.Infof("Registering default sub-checks")

	// Only register Docker-related checks if logs agent is enabled
	if c.isLogsAgentEnabled() {
		log.Infof("Logs agent is enabled, registering Docker permissions check")
		if err := c.RegisterSubCheck(NewDockerPermissionsCheck()); err != nil {
			log.Warnf("Failed to register Docker permissions check: %v", err)
		} else {
			log.Infof("Successfully registered Docker permissions check")
		}
	} else {
		log.Infof("Logs agent is not enabled, skipping Docker permissions check registration")
	}

	log.Infof("Default sub-checks registration completed")
}

// isLogsAgentEnabled checks if the logs agent is enabled in the configuration
func (c *component) isLogsAgentEnabled() bool {
	log.Infof("Checking if logs agent is enabled")

	if c.cfg == nil {
		log.Infof("Config is nil, logs agent considered disabled")
		return false
	}

	logsConfig := c.cfg.GetStringMap("logs")
	logsEnabled := c.cfg.GetBool("logs_enabled")
	logEnabled := c.cfg.GetBool("log_enabled")

	log.Infof("Logs agent configuration - logs config length: %d, logs_enabled: %t, log_enabled: %t",
		len(logsConfig), logsEnabled, logEnabled)

	isEnabled := len(logsConfig) > 0 || logsEnabled || logEnabled
	log.Infof("Logs agent enabled: %t", isEnabled)

	return isEnabled
}

// CheckHealth performs health checks related to logs agent health
func (c *component) CheckHealth(ctx context.Context) ([]healthplatform.Issue, error) {
	log.Infof("Starting logs agent health check")

	if ctx == nil {
		log.Warnf("Context is nil, using background context")
		ctx = context.Background()
	}

	c.subChecksMu.RLock()
	defer c.subChecksMu.RUnlock()

	subChecksCount := len(c.subChecks)
	log.Infof("Running health check with %d registered sub-checks", subChecksCount)

	if subChecksCount == 0 {
		log.Infof("No sub-checks registered, returning empty issues list")
		return []healthplatform.Issue{}, nil
	}

	var allIssues []healthplatform.Issue
	var failedChecks int
	var successfulChecks int

	// Run all registered sub-checks
	for i, check := range c.subChecks {
		if check == nil {
			log.Warnf("Sub-check at index %d is nil, skipping", i)
			failedChecks++
			continue
		}

		checkName := check.Name()
		log.Infof("Running sub-check %d/%d: %s", i+1, subChecksCount, checkName)

		issues, err := check.Check(ctx)
		if err != nil {
			log.Warnf("Sub-check '%s' failed: %v", checkName, err)
			failedChecks++
			continue
		}

		issuesCount := len(issues)
		log.Infof("Sub-check '%s' completed successfully with %d issues", checkName, issuesCount)

		if issuesCount > 0 {
			log.Infof("Issues from sub-check '%s': %+v", checkName, issues)
		}

		allIssues = append(allIssues, issues...)
		successfulChecks++
	}

	totalIssues := len(allIssues)
	log.Infof("Health check completed - successful: %d, failed: %d, total issues: %d",
		successfulChecks, failedChecks, totalIssues)

	return allIssues, nil
}

// RegisterSubCheck registers a new health check sub-component
func (c *component) RegisterSubCheck(check logsagenthealth.SubCheck) error {
	log.Infof("Attempting to register sub-check")

	if check == nil {
		log.Errorf("Cannot register nil sub-check")
		return fmt.Errorf("sub-check cannot be nil")
	}

	checkName := check.Name()
	log.Infof("Registering sub-check: %s", checkName)

	c.subChecksMu.Lock()
	defer c.subChecksMu.Unlock()

	previousCount := len(c.subChecks)
	c.subChecks = append(c.subChecks, check)
	newCount := len(c.subChecks)

	log.Infof("Sub-check '%s' registered successfully. Previous count: %d, new count: %d",
		checkName, previousCount, newCount)

	return nil
}
