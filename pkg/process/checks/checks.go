// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package checks

import (
	model "github.com/DataDog/agent-payload/v5/process"

	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Name for check performed by process-agent or system-probe
const (
	ProcessCheckName       = "process"
	RTProcessCheckName     = "rtprocess"
	ContainerCheckName     = "container"
	RTContainerCheckName   = "rtcontainer"
	ConnectionsCheckName   = "connections"
	DiscoveryCheckName     = "process_discovery"
	ProcessEventsCheckName = "process_events"
)

// SysProbeConfig provides access to system probe configuration
type SysProbeConfig struct {
	MaxConnsPerMessage int
	// System probe collection configuration
	SystemProbeAddress string
	// System probe process module on/off configuration
	ProcessModuleEnabled bool
}

// Check is an interface for Agent checks that collect data. Each check returns
// a specific MessageBody type that will be published to the intake endpoint or
// processed in another way (e.g. printed for debugging).
// Before checks are used you must called Init.
type Check interface {
	// Name returns the name of the check
	Name() string
	// IsEnabled returns true if the check is enabled by configuration
	IsEnabled() bool
	// Realtime indicates if this check only runs in real-time mode
	Realtime() bool
	// Init initializes the check
	Init(syscfg *SysProbeConfig, info *HostInfo, oneShot bool) error
	// SupportsRunOptions returns true if the check supports RunOptions
	SupportsRunOptions() bool
	// Run runs the check
	Run(nextGroupID func() int32, options *RunOptions) (RunResult, error)
	// Cleanup performs resource cleanup after check is no longer running
	Cleanup()
	// ShouldSaveLastRun saves results of the last run
	ShouldSaveLastRun() bool
}

// RunOptions provides run options for checks
type RunOptions struct {
	RunStandard bool
	RunRealtime bool
}

// RunResult is a result for a check run
type RunResult interface {
	Payloads() []model.MessageBody
	RealtimePayloads() []model.MessageBody
}

// StandardRunResult is a run result containing payloads for standard run
type StandardRunResult []model.MessageBody

//nolint:revive // TODO(PROC) Fix revive linter
func (p StandardRunResult) Payloads() []model.MessageBody {
	return p
}

//nolint:revive // TODO(PROC) Fix revive linter
func (p StandardRunResult) RealtimePayloads() []model.MessageBody {
	return nil
}

// CombinedRunResult is a run result containing payloads for standard and realtime runs
type CombinedRunResult struct {
	Standard []model.MessageBody
	Realtime []model.MessageBody
}

//nolint:revive // TODO(PROC) Fix revive linter
func (p CombinedRunResult) Payloads() []model.MessageBody {
	return p.Standard
}

//nolint:revive // TODO(PROC) Fix revive linter
func (p CombinedRunResult) RealtimePayloads() []model.MessageBody {
	return p.Realtime
}

// All is a list of all runnable checks. Putting a check in here does not guarantee it will be run,
// it just guarantees that the collector will be able to find the check.
// If you want to add a check you MUST register it here.
func All(config, sysprobeYamlCfg ddconfig.ReaderWriter, syscfg *sysconfigtypes.Config, wmeta workloadmeta.Component) []Check {
	return []Check{
		NewProcessCheck(config, sysprobeYamlCfg, wmeta),
		NewContainerCheck(config, wmeta),
		NewRTContainerCheck(config, wmeta),
		NewConnectionsCheck(config, sysprobeYamlCfg, syscfg, wmeta),
		NewProcessDiscoveryCheck(config),
		NewProcessEventsCheck(config),
	}
}

// RTName returns the name of the corresponding realtime check
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

func canEnableContainerChecks(config ddconfig.Reader, displayFeatureWarning bool) bool {
	// The process and container checks are mutually exclusive
	if config.GetBool("process_config.process_collection.enabled") {
		return false
	}
	if !ddconfig.IsAnyContainerFeaturePresent() {
		if displayFeatureWarning {
			_ = log.Warn("Disabled container checks because no container environment detected (see list of detected features in `agent status`)")
		}
		return false
	}

	return config.GetBool("process_config.container_collection.enabled")
}
