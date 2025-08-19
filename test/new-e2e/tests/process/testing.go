// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package process contains end-to-end tests for the general functionality of the process agent.
package process

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

//go:embed config/process_check.yaml
var processCheckConfigStr string

//go:embed config/process_discovery_check.yaml
var processDiscoveryCheckConfigStr string

//go:embed config/process_check_in_core_agent.yaml
var processCheckInCoreAgentConfigStr string

//go:embed config/process_check_in_core_agent_wlm_process_collector.yaml
var processCheckInCoreAgentWLMProcessCollectorConfigStr string

//go:embed config/system_probe.yaml
var systemProbeConfigStr string

//go:embed config/npm.yaml
var systemProbeNPMConfigStr string

//go:embed compose/fake-process-compose.yaml
var fakeProcessCompose string

//go:embed config/process_agent_refresh_nix.yaml
var processAgentRefreshStr string

//go:embed config/core_agent_refresh_nix.yaml
var coreAgentRefreshStr string

//go:embed config/process_agent_refresh_win.yaml
var processAgentWinRefreshStr string

// AgentStatus is a subset of the agent's status response for asserting the process-agent runtime
type AgentStatus struct {
	ProcessAgentStatus struct {
		Expvars struct {
			Map struct {
				EnabledChecks                []string            `json:"enabled_checks"`
				SysProbeProcessModuleEnabled bool                `json:"system_probe_process_module_enabled"`
				Endpoints                    map[string][]string `json:"endpoints"`
			} `json:"process_agent"`
		} `json:"expvars"`
		Error string `json:"error"`
	} `json:"processAgentStatus"`
	ProcessComponentStatus struct {
		Expvars struct {
			Map struct {
				EnabledChecks                []string            `json:"enabled_checks"`
				SysProbeProcessModuleEnabled bool                `json:"system_probe_process_module_enabled"`
				Endpoints                    map[string][]string `json:"endpoints"`
			} `json:"process_agent"`
		} `json:"expvars"`
	} `json:"processComponentStatus"`
}

func getAgentStatus(t *assert.CollectT, client agentclient.Agent) AgentStatus {
	status := client.Status(agentclient.WithArgs([]string{"--json"}))
	assert.NotNil(t, status, "failed to get agent status")

	var statusMap AgentStatus
	err := json.Unmarshal([]byte(status.Content), &statusMap)
	assert.NoError(t, err, "failed to unmarshal agent status")

	return statusMap
}

// assertRunningChecks asserts that the given process agent checks are running on the given VM
func assertRunningChecks(t *assert.CollectT, client agentclient.Agent, checks []string, withSystemProbe bool) {
	statusMap := getAgentStatus(t, client)

	assert.ElementsMatch(t, checks, statusMap.ProcessAgentStatus.Expvars.Map.EnabledChecks)

	if withSystemProbe {
		assert.True(t, statusMap.ProcessAgentStatus.Expvars.Map.SysProbeProcessModuleEnabled,
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

// assertProcessCollectedNew asserts that the given process is collected by the process check
// and that it has the expected data populated
// This is a new function to replace assertProcessCollected, but we need to verify it actually reduces the flakiness
// of test runs before we fully switch over.
func assertProcessCollectedNew(
	t require.TestingT, payloads []*aggregator.ProcessPayload, withIOStats bool, process string,
) {
	// Find Processes
	var procs []*agentmodel.Process
	for _, payload := range payloads {
		procs = append(procs, filterProcesses(process, payload.Processes)...)
	}
	require.NotEmpty(t, procs, "'%s' process not found in payloads: \n%+v", process, payloads)

	assertProcesses(t, procs, withIOStats, process)
}

func assertProcessCommandLineArgs(t require.TestingT, processes []*agentmodel.Process, processCMDArgs []string) {
	for _, proc := range processes {
		// command arguments include the first command/program which can differ depending on the path,
		// so we compare the user provided arguments starting from index 1
		assert.Equalf(t, proc.Command.Args[1:], processCMDArgs[1:], "process args do not match. Expected %+v", processCMDArgs)
	}
}

// assertProcesses asserts that the given processes are collected by the process check
func assertProcesses(t require.TestingT, procs []*agentmodel.Process, withIOStats bool, process string) {
	// verify process data is populated
	var hasData bool
	for _, proc := range procs {
		if hasData = processHasData(proc); hasData {
			break
		}
	}
	assert.True(t, hasData, "'%s' process does not have all data populated in: %+v", process, procs)

	// verify IO stats are populated
	if withIOStats {
		var hasIOStats bool
		for _, proc := range procs {
			if hasIOStats = processHasIOStats(proc); hasIOStats {
				break
			}
		}
		assert.True(t, hasIOStats, "'%s' process does not have IO stats populated in %+v", process, procs)
	}
}

// assertContainersCollectedNew asserts that the given containers are collected
func assertContainersCollectedNew(t assert.TestingT, payloads []*aggregator.ProcessPayload, expectedContainers []string) {
	for _, container := range expectedContainers {
		var found bool
		for _, payload := range payloads {
			if findContainer(container, payload.Containers) {
				found = true
				break
			}
		}
		assert.True(t, found, "%s container not found in payloads: %+v", container, payloads)
	}
}

// requireProcessNotCollected asserts that the given process is NOT collected by the process check
func requireProcessNotCollected(t require.TestingT, payloads []*aggregator.ProcessPayload, process string) {
	for _, payload := range payloads {
		require.Empty(t, filterProcesses(process, payload.Processes))
	}
}

// findProcess returns whether the process with the given name exists in the given list of
// processes and whether it has the expected data populated
func findProcess(
	name string, processes []*agentmodel.Process, withIOStats bool,
) (found, populated bool) {
	for _, process := range processes {
		if matchProcess(process, name) {
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

func filterProcessPayloadsByName(payloads []*aggregator.ProcessPayload, processName string) []*agentmodel.Process {
	var procs []*agentmodel.Process
	for _, payload := range payloads {
		procs = append(procs, filterProcesses(processName, payload.Processes)...)
	}
	return procs
}

// filterProcesses returns processes which match the given process name
func filterProcesses(name string, processes []*agentmodel.Process) []*agentmodel.Process {
	var matched []*agentmodel.Process
	for _, process := range processes {
		if matchProcess(process, name) {
			matched = append(matched, process)
		}
	}
	return matched
}

// matchProcess returns whether the given process matches the given name in the Args or Exe
func matchProcess(process *agentmodel.Process, name string) bool {
	return len(process.Command.Args) > 0 &&
		(process.Command.Args[0] == name || process.Command.Exe == name)
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

// assertContainersCollected asserts that the given containers are collected
func assertContainersCollected(t *testing.T, payloads []*aggregator.ProcessPayload, expectedContainers []string) {
	defer func() {
		if t.Failed() {
			t.Logf("Payloads:\n%+v\n", payloads)
		}
	}()

	for _, container := range expectedContainers {
		var found bool
		for _, payload := range payloads {
			if findContainer(container, payload.Containers) {
				found = true
				break
			}
		}
		assert.True(t, found, "%s container not found", container)
	}
}

// assertContainersNotCollected asserts that the given containers are not collected
func assertContainersNotCollected(t *testing.T, payloads []*aggregator.ProcessPayload, containers []string) {
	for _, container := range containers {
		var found bool
		for _, payload := range payloads {
			if findContainer(container, payload.Containers) {
				found = true
				t.Logf("Payload:\n%+v\n", payload)
				break
			}
		}
		assert.False(t, found, "%s container found", container)
	}
}

// findContainer returns whether the container with the given name exists in the given list of
// containers and whether it has the expected data populated
func findContainer(name string, containers []*agentmodel.Container) bool {
	// check if there is a tag for the container. The tag could be `container_name:*` or `short_image:*`
	containerNameTag := fmt.Sprintf(":%s", name)
	for _, container := range containers {
		for _, tag := range container.Tags {
			if strings.HasSuffix(tag, containerNameTag) {
				return true
			}
		}
	}
	return false
}

// assertManualProcessCheck asserts that the given process is collected and reported in the output
// of the manual process check
func assertManualProcessCheck(t require.TestingT, check string, withIOStats bool, process string, expectedContainers ...string) {
	var checkOutput struct {
		Processes []*agentmodel.Process `json:"processes"`
	}

	err := json.Unmarshal([]byte(check), &checkOutput)
	require.NoError(t, err, "failed to unmarshal process check output")

	procs := filterProcesses(process, checkOutput.Processes)
	require.NotEmpty(t, procs, "'%s' process not found in check:\n%s\n", process, check)

	assertProcesses(t, procs, withIOStats, process)
	assertManualContainerCheck(t, check, expectedContainers...)
}

// assertManualContainerCheck asserts that the given container is collected from a manual container check
func assertManualContainerCheck(t require.TestingT, check string, expectedContainers ...string) {
	var checkOutput struct {
		Containers []*agentmodel.Container `json:"containers"`
	}

	err := json.Unmarshal([]byte(check), &checkOutput)
	require.NoError(t, err, "failed to unmarshal process check output")

	for _, container := range expectedContainers {
		assert.Truef(t, findContainer(container, checkOutput.Containers),
			"%s container not found in %+v", container, checkOutput.Containers)
	}
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

func assertAPIKeyStatus(collect *assert.CollectT, apiKey string, agentClient agentclient.Agent, coreAgent bool) {
	// Assert that the status has the correct API key
	statusMap := getAgentStatus(collect, agentClient)
	endpoints := statusMap.ProcessAgentStatus.Expvars.Map.Endpoints
	if coreAgent {
		endpoints = statusMap.ProcessComponentStatus.Expvars.Map.Endpoints
	}
	found := false
	for _, epKeys := range endpoints {
		for _, key := range epKeys {
			// Original key is obfuscated to the last 5 characters
			if key == apiKey[len(apiKey)-5:] {
				found = true
				break
			}
		}
	}
	require.True(collect, found, "API key %s not found in endpoints %+v", apiKey, endpoints)
}

func assertLastPayloadAPIKey(collect *assert.CollectT, expectedAPIKey string, fakeIntakeClient *fakeintakeclient.Client) {
	// Assert that the last received payload has the correct API key
	lastAPIKey, err := fakeIntakeClient.GetLastProcessPayloadAPIKey()
	require.NoError(collect, err)
	assert.Equal(collect, expectedAPIKey, lastAPIKey)
}

func assertAllPayloadsAPIKeys(collect *assert.CollectT, expectedAPIKeys []string, fakeIntakeClient *fakeintakeclient.Client) {
	// Assert that all received payloads have the expected API keys
	payloadKeys, err := fakeIntakeClient.GetAllProcessPayloadAPIKeys()
	require.NoError(collect, err)
	for _, expectedAPIKey := range expectedAPIKeys {
		assert.Contains(collect, payloadKeys, expectedAPIKey)
	}
}
