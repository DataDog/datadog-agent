// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	model "github.com/DataDog/agent-payload/v5/process"
)

// Name for check performed by process-agent or system-probe
const (
	ProcessCheckName       = "process"
	RTProcessCheckName     = "rtprocess"
	ContainerCheckName     = "container"
	RTContainerCheckName   = "rtcontainer"
	ConnectionsCheckName   = "connections"
	PodCheckName           = "pod"
	PodCheckManifestName   = "pod_manifest"
	DiscoveryCheckName     = "process_discovery"
	ProcessEventsCheckName = "process_events"
)

// SysProbeConfig provides access to system probe configuration
type SysProbeConfig struct {
	MaxConnsPerMessage int
	// System probe collection configuration
	SystemProbeAddress string
}

// Check is an interface for Agent checks that collect data. Each check returns
// a specific MessageBody type that will be published to the intake endpoint or
// processed in another way (e.g. printed for debugging).
// Before checks are used you must called Init.
type Check interface {
	Name() string
	IsEnabled() bool
	Realtime() bool
	Init(syscfg *SysProbeConfig, info *HostInfo) error
	SupportsRunOptions() bool
	Run(nextGroupID func() int32, options *RunOptions) (RunResult, error)
	Cleanup()
	ShouldSaveLastRun() bool
}

type RunOptions struct {
	RunStandard bool
	RunRealtime bool
}

type RunResult interface {
	Payloads() []model.MessageBody
	RealtimePayloads() []model.MessageBody
}

// StandardRunResult is a run result containing payloads for standard run
type StandardRunResult []model.MessageBody

func (p StandardRunResult) Payloads() []model.MessageBody {
	return p
}

func (p StandardRunResult) RealtimePayloads() []model.MessageBody {
	return nil
}

// CombinedRunResult is a run result containing payloads for standard and realtime runs
type CombinedRunResult struct {
	Standard []model.MessageBody
	Realtime []model.MessageBody
}

func (p CombinedRunResult) Payloads() []model.MessageBody {
	return p.Standard
}

func (p CombinedRunResult) RealtimePayloads() []model.MessageBody {
	return p.Realtime
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

// RTName returns the name of corresponding realtime check
func RTName(checkName string) string {
	switch checkName {
	case ProcessCheckName:
		return RTProcessCheckName
	case ContainerCheckName:
		return RTContainerCheckName
	default:
		return ""
	}
}
