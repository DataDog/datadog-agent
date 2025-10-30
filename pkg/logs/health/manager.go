// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package health provides a health manager for the logs agent to coordinate the suite of logs-specific checks
package health

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
	"github.com/DataDog/datadog-agent/pkg/logs/health/checks"
)

// Manager coordinates the health checks for the logs agent
type Manager struct {
	log            log.Component
	cfg            config.Component
	healthPlatform healthplatform.Component
}

// NewManager creates a new health manager for the logs agent
func NewManager(log log.Component, cfg config.Component, healthPlatform healthplatform.Component) *Manager {
	return &Manager{
		log:            log,
		cfg:            cfg,
		healthPlatform: healthPlatform,
	}
}

// Start starts the health manager
func (h *Manager) Start() {
	if h == nil {
		return
	}
	if h.cfg.GetBool("logs_config.docker_container_use_file") {
		err := h.healthPlatform.RegisterCheck(checks.NewDockerLogPermissionsCheckConfig())
		if err != nil {
			h.log.Error("Failed to register Docker log permissions check: ", err)
			return
		}
	}
}

// Stop stops the health manager
func (h *Manager) Stop() {}
