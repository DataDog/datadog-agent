// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package process contains end-to-end tests for the general functionality of the process agent.
package process

import (
	_ "embed"
	"encoding/json"
	"testing"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
)

//go:embed config/process_check.yaml
var processCheckConfigStr string

//go:embed config/process_discovery_check.yaml
var processDiscoveryCheckConfigStr string

//go:embed config/system_probe.yaml
var systemProbeConfigStr string

// assertRunningChecks asserts that the given process agent checks are running on the given VM
func assertRunningChecks(t *assert.CollectT, vm client.VM, checks []string, withSystemProbe bool) {
	status := vm.Execute("sudo datadog-agent status --json")
	var statusMap struct {
		ProcessAgentStatus struct {
			Expvars struct {
				EnabledChecks                []string `json:"enabled_checks"`
				SysProbeProcessModuleEnabled bool     `json:"system_probe_process_module_enabled"`
			} `json:"expvars"`
		} `json:"processAgentStatus"`
	}
	err := json.Unmarshal([]byte(status), &statusMap)
	assert.NoError(t, err, "failed to unmarshal agent status")

	assert.ElementsMatch(t, checks, statusMap.ProcessAgentStatus.Expvars.EnabledChecks)

	if withSystemProbe {
		assert.True(t, statusMap.ProcessAgentStatus.Expvars.SysProbeProcessModuleEnabled,
			"system probe process module not enabled")
	}
}

// assertStressProcessCollected asserts that the stress process is collected by the process check
// and that it has the expected data populated
func assertStressProcessCollected(
	t *testing.T, payloads []*aggregator.ProcessPayload, withIOStats bool,
) {
	var found, populated bool
	for _, payload := range payloads {
		for _, process := range payload.Processes {
			if len(process.Command.Args) > 0 && process.Command.Args[0] == "stress" {
				found = true
				populated = processHasData(process)

				if withIOStats {
					populated = populated && processHasIOStats(process)
				}

				if populated {
					break
				}
			}
		}

		if found && populated {
			break
		}
	}

	assert.True(t, found, "stress process not found")
	assert.True(t, populated, "no stress process had all data populated")
}

// processHasData asserts that the given process has the expected data populated
func processHasData(process *agentmodel.Process) bool {
	return process.Pid != 0 && process.NsPid != 0 && len(process.User.Name) > 0 &&
		process.Cpu.TotalPct > 0 && process.Cpu.UserPct > 0 && process.Cpu.SystemPct > 0 &&
		process.Memory.Rss > 0 && process.Memory.Vms > 0
}

// processHasIOStats asserts that the given process has the expected IO stats populated
func processHasIOStats(process *agentmodel.Process) bool {
	// the stress process only writes to disk, does not read from it
	return process.IoStat.WriteRate > 0 && process.IoStat.WriteBytesRate > 0
}

// assertStressProcessDiscoveryCollected asserts that the stress process is collected by the process
// discovery check and that it has the expected data populated
func assertStressProcessDiscoveryCollected(
	t *testing.T, payloads []*aggregator.ProcessDiscoveryPayload,
) {
	var found, populated bool
	for _, payload := range payloads {
		for _, disc := range payload.ProcessDiscoveries {
			if len(disc.Command.Args) > 0 && disc.Command.Args[0] == "stress" {
				found = true
				populated = processDiscoveryHasData(disc)
				if populated {
					break
				}
			}
		}

		if found && populated {
			break
		}
	}

	assert.True(t, found, "stress process not found")
	assert.True(t, populated, "no stress process had all data populated")
}

// processDiscoveryHasData asserts that the given process discovery has the expected data populated
func processDiscoveryHasData(disc *agentmodel.ProcessDiscovery) bool {
	return disc.Pid != 0 && disc.NsPid != 0 && len(disc.User.Name) > 0
}
