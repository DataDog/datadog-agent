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
	if deps.Config == nil {
		log.Warnf("Config dependency is nil, logs agent health checker may not function properly")
	}

	comp := &component{
		cfg:       deps.Config,
		subChecks: make([]logsagenthealth.SubCheck, 0),
	}

	// Register default sub-checks
	comp.registerDefaultSubChecks()

	return comp
}

// registerDefaultSubChecks registers the default set of health checks
func (c *component) registerDefaultSubChecks() {
	// Only register Docker-related checks if logs agent is enabled
	if c.isLogsAgentEnabled() {
		if err := c.RegisterSubCheck(NewDockerPermissionsCheck()); err != nil {
			log.Warnf("Failed to register Docker permissions check: %v", err)
		}
	}
}

// isLogsAgentEnabled checks if the logs agent is enabled in the configuration
func (c *component) isLogsAgentEnabled() bool {
	if c.cfg == nil {
		return false
	}

	logsConfig := c.cfg.GetStringMap("logs")
	logsEnabled := c.cfg.GetBool("logs_enabled")
	logEnabled := c.cfg.GetBool("log_enabled")

	return len(logsConfig) > 0 || logsEnabled || logEnabled
}

// CheckHealth performs health checks related to logs agent health
func (c *component) CheckHealth(ctx context.Context) ([]healthplatform.Issue, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	c.subChecksMu.RLock()
	defer c.subChecksMu.RUnlock()

	if len(c.subChecks) == 0 {
		return []healthplatform.Issue{}, nil
	}

	var allIssues []healthplatform.Issue

	// Run all registered sub-checks
	for _, check := range c.subChecks {
		if check == nil {
			continue
		}
		log.Infof("Running sub-check '%s'", check.Name())
		issues, err := check.Check(ctx)
		if err != nil {
			log.Warnf("Sub-check '%s' failed: %v", check.Name(), err)
			continue
		}

		allIssues = append(allIssues, issues...)
	}

	return allIssues, nil
}

// RegisterSubCheck registers a new health check sub-component
func (c *component) RegisterSubCheck(check logsagenthealth.SubCheck) error {
	if check == nil {
		return fmt.Errorf("sub-check cannot be nil")
	}

	c.subChecksMu.Lock()
	defer c.subChecksMu.Unlock()

	c.subChecks = append(c.subChecks, check)

	return nil
}
