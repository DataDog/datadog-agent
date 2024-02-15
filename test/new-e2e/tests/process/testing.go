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
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

//go:embed config/process_check.yaml
var processCheckConfigStr string

//go:embed config/process_discovery_check.yaml
var processDiscoveryCheckConfigStr string

//go:embed config/process_check_in_core_agent.yaml
var processCheckInCoreAgentConfigStr string

//go:embed config/system_probe.yaml
var systemProbeConfigStr string

// assertRunningChecks asserts that the given process agent checks are running on the given VM
func assertRunningChecks(t *assert.CollectT, vm *components.RemoteHost, checks []string, withSystemProbe bool, command string) {
	status := vm.MustExecute(command)
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

// assertProcessCollected asserts that the given process is collected by the process check
// and that it has the expected data populated
func assertProcessCollected(
	t *testing.T, payloads []*aggregator.ProcessPayload, withIOStats bool, process string,
) {
	defer func() {
		if t.Failed() {
			t.Logf("Payloads:\n%+v\n", payloads)
		}
	}()

	var found, populated bool
	for _, payload := range payloads {
		found, populated = findProcess(process, payload.Processes, withIOStats)
		if found && populated {
			break
		}
	}

	require.True(t, found, "%s process not found", process)
	assert.True(t, populated, "no %s process had all data populated", process)
}

// assertProcessNotCollected asserts that the given process is NOT collected by the process check
func assertProcessNotCollected(t *testing.T, payloads []*aggregator.ProcessPayload, process string) {
	for _, payload := range payloads {
		found, _ := findProcess(process, payload.Processes, false)
		require.False(t, found, "%s process found", process)
	}
}

// findProcess returns whether the process with the given name exists in the given list of
// processes and whether it has the expected data populated
func findProcess(
	name string, processes []*agentmodel.Process, withIOStats bool,
) (found, populated bool) {
	for _, process := range processes {
		if len(process.Command.Args) > 0 && process.Command.Args[0] == name {
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

	return found, populated
}

// processHasData asserts that the given process has the expected data populated
func processHasData(process *agentmodel.Process) bool {
	return process.Pid != 0 && process.Command.Ppid != 0 && len(process.User.Name) > 0 &&
		(process.Cpu.UserPct > 0 || process.Cpu.SystemPct > 0) &&
		(process.Memory.Rss > 0 || process.Memory.Vms > 0 || process.Memory.Swap > 0)
}

// processHasIOStats asserts that the given process has the expected IO stats populated
func processHasIOStats(process *agentmodel.Process) bool {
	// The processes we currently use to test can only read or write, not both
	return process.IoStat.WriteRate > 0 && process.IoStat.WriteBytesRate > 0 || process.IoStat.ReadRate > 0 && process.IoStat.ReadBytesRate > 0
}

// assertProcessDiscoveryCollected asserts that the given process is collected by the process
// discovery check and that it has the expected data populated
func assertProcessDiscoveryCollected(
	t *testing.T, payloads []*aggregator.ProcessDiscoveryPayload, process string,
) {
	defer func() {
		if t.Failed() {
			t.Logf("Payloads:\n%+v\n", payloads)
		}
	}()

	var found, populated bool
	for _, payload := range payloads {
		found, populated = findProcessDiscovery(process, payload.ProcessDiscoveries)
		if found && populated {
			break
		}
	}

	require.True(t, found, "%s process not found", process)
	assert.True(t, populated, "no %s process had all data populated", process)
}

// findProcessDiscovery returns whether the process with the given name exists in the given list of
// process discovery payloads and whether it has the expected data populated
func findProcessDiscovery(
	name string, discs []*agentmodel.ProcessDiscovery,
) (found, populated bool) {
	for _, disc := range discs {
		if len(disc.Command.Args) > 0 && disc.Command.Args[0] == name {
			found = true
			populated = processDiscoveryHasData(disc)
			if populated {
				break
			}
		}
	}

	return found, populated
}

// processDiscoveryHasData asserts that the given process discovery has the expected data populated
func processDiscoveryHasData(disc *agentmodel.ProcessDiscovery) bool {
	return disc.Pid != 0 && disc.Command.Ppid != 0 && len(disc.User.Name) > 0
}

// assertManualProcessCheck asserts that the given process is collected and reported in the output
// of the manual process check
func assertManualProcessCheck(t *testing.T, check string, withIOStats bool, process string) {
	defer func() {
		if t.Failed() {
			t.Logf("Check output:\n%s\n", check)
		}
	}()

	var checkOutput struct {
		Processes []*agentmodel.Process `json:"processes"`
	}
	err := json.Unmarshal([]byte(check), &checkOutput)
	require.NoError(t, err, "failed to unmarshal process check output")

	found, populated := findProcess(process, checkOutput.Processes, withIOStats)

	require.True(t, found, "%s process not found", process)
	assert.True(t, populated, "no %s process had all data populated", process)
}

// assertManualProcessDiscoveryCheck asserts that the given process is collected and reported in
// the output of the manual process_discovery check
func assertManualProcessDiscoveryCheck(t *testing.T, check string, process string) {
	defer func() {
		if t.Failed() {
			t.Logf("Check output:\n%s\n", check)
		}
	}()

	var checkOutput struct {
		ProcessDiscoveries []*agentmodel.ProcessDiscovery `json:"processDiscoveries"`
	}
	err := json.Unmarshal([]byte(check), &checkOutput)
	require.NoError(t, err, "failed to unmarshal process check output")

	found, populated := findProcessDiscovery(process, checkOutput.ProcessDiscoveries)

	require.True(t, found, "%s process not found", process)
	assert.True(t, populated, "no %s process had all data populated", process)
}
