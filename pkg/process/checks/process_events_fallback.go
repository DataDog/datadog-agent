// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux
// +build !linux

package checks

import (
	"errors"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
)

// ProcessEvents is a ProcessEventsCheck singleton
var ProcessEvents = &ProcessEventsCheck{}

// ProcessEventsCheck collects process lifecycle events such as exec and exit signals
type ProcessEventsCheck struct {
}

// Init initializes the ProcessEventsCheck.
func (e *ProcessEventsCheck) Init(_ *config.AgentConfig, info *model.SystemInfo) {
}

// Name returns the name of the ProcessEventsCheck.
func (e *ProcessEventsCheck) Name() string { return config.ProcessEventsCheckName }

// RealTime returns a value that says whether this check should be run in real time.
func (e *ProcessEventsCheck) RealTime() bool { return false }

// Run fetches process lifecycle events that have been stored in-memory since the last check run
func (e *ProcessEventsCheck) Run(cfg *config.AgentConfig, groupID int32) ([]model.MessageBody, error) {
	return nil, errors.New("the process_events check is not supported on this system")
}

// Cleanup frees any resource held by the ProcessEventsCheck before the agent exits
func (e *ProcessEventsCheck) Cleanup() {}
