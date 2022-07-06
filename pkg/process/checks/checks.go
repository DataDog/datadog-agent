// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
)

// Check is an interface for Agent checks that collect data. Each check returns
// a specific MessageBody type that will be published to the intake endpoint or
// processed in another way (e.g. printed for debugging).
// Before checks are used you must called Init.
type Check interface {
	Init(cfg *config.AgentConfig, info *model.SystemInfo)
	Name() string
	RealTime() bool
	Run(cfg *config.AgentConfig, groupID int32) ([]model.MessageBody, error)
	Cleanup()
}

// RunOptions provides run options for checks
type RunOptions struct {
	RunStandard bool
	RunRealTime bool
}

// RunResult is a result for a check run
type RunResult struct {
	Standard []model.MessageBody
	RealTime []model.MessageBody
}

// CheckWithRealTime provides an extended interface for running composite checks
type CheckWithRealTime interface {
	Check
	RealTimeName() string
	RunWithOptions(cfg *config.AgentConfig, nextGroupID func() int32, options RunOptions) (*RunResult, error)
}

// All is a list of all runnable checks. Putting a check in here does not guarantee it will be run,
// it just guarantees that the collector will be able to find the check.
// If you want to add a check you MUST register it here.
var All = []Check{
	Process,
	Container,
	RTContainer,
	Connections,
	Pod,
	ProcessDiscovery,
	ProcessEvents,
}
