// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !linux !orchestrator

package checks

import (
	"time"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
)

// Pod is a singleton PodCheck.
var Pod = &PodCheck{}

// PodCheck is a check that returns container metadata and stats.
type PodCheck struct {
	sysInfo *model.SystemInfo
	lastRun time.Time
}

// Init initializes a ContainerCheck instance.
func (c *PodCheck) Init(cfg *config.AgentConfig, info *model.SystemInfo) {
	c.sysInfo = info
}

// Name returns the name of the ProcessCheck.
func (c *PodCheck) Name() string { return "container" }

// Endpoint returns the endpoint where this check is submitted.
func (c *PodCheck) Endpoint() string { return "/api/v1/orchestrator" }

// RealTime indicates if this check only runs in real-time mode.
func (c *PodCheck) RealTime() bool { return false }

// Run runs the PodCheck to collect a list of running containers and the
// stats for each container.
func (c *PodCheck) Run(cfg *config.AgentConfig, groupID int32) ([]model.MessageBody, error) {
	return nil, nil
}
