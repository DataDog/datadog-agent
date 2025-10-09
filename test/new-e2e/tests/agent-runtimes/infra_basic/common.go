// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package infrabasic contains end-to-end tests for infrastructure basic mode functionality.
// These tests verify that core system checks work correctly when the agent is configured
// to run in basic infrastructure mode, which only instantiates essential components.
package infrabasic

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/testcommon/check"
	agentclient "github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

// ============================================================================
// Type Definitions
// ============================================================================

type infraBasicSuite struct { //nolint:unused
	e2e.BaseSuite[environments.Host]
	descriptor e2eos.Descriptor
}

// RunnerStats represents the check runner statistics from agent status
type RunnerStats struct {
	CheckName         string `json:"CheckName"`
	TotalRuns         uint64 `json:"TotalRuns"`
	TotalErrors       uint64 `json:"TotalErrors"`
	TotalWarnings     uint64 `json:"TotalWarnings"`
	CheckID           string `json:"CheckID"`
	CheckConfigSource string `json:"CheckConfigSource"`
}

// AgentStatusJSON represents the relevant parts of agent status JSON output
type AgentStatusJSON struct {
	RunnerStats map[string]RunnerStats `json:"runnerStats"`
}

// ============================================================================
// Utility Functions
// ============================================================================

func (s *infraBasicSuite) getSuiteOptions() []e2e.SuiteOption { //nolint:unused
	suiteOptions := []e2e.SuiteOption{}
	suiteOptions = append(suiteOptions, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithEC2InstanceOptions(ec2.WithOS(s.descriptor), ec2.WithInstanceType("t3.micro")),
		),
	))

	return suiteOptions
}

// getRunnerStats retrieves the runner statistics from the agent status
func (s *infraBasicSuite) getRunnerStats() (map[string]RunnerStats, error) { //nolint:unused
	status := s.Env().Agent.Client.Status(agentclient.WithArgs([]string{"--json"}))

	var statusMap AgentStatusJSON
	err := json.Unmarshal([]byte(status.Content), &statusMap)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal agent status: %w", err)
	}

	return statusMap.RunnerStats, nil
}

// isCheckScheduled returns true if the check is scheduled and has run at least once
func (s *infraBasicSuite) isCheckScheduled(checkName string, stats map[string]RunnerStats) bool { //nolint:unused
	for _, stat := range stats {
		if stat.CheckName == checkName && stat.TotalRuns > 0 {
			return true
		}
	}
	return false
}

// verifyCheckRunsWithConfig runs a check and verifies it executed successfully by checking TotalRuns > 0
func (s *infraBasicSuite) verifyCheckRunsWithConfig(checkName string, checkConfig string, agentConfig string) bool { //nolint:unused
	env := s.Env()
	host := env.RemoteHost

	// Write agent config
	tmpFolder, err := host.GetTmpFolder()
	if err != nil {
		s.T().Fatalf("Failed to get tmp folder: %v", err)
	}

	confFolder := "/etc/datadog-agent"
	if s.descriptor.Family() == e2eos.WindowsFamily {
		confFolder = "C:\\ProgramData\\Datadog"
	}

	extraConfigFilePath := host.JoinPath(tmpFolder, "datadog.yaml")
	_, err = host.WriteFile(extraConfigFilePath, []byte(agentConfig))
	if err != nil {
		s.T().Fatalf("Failed to write agent config: %v", err)
	}

	// Write check config
	tmpCheckConfigFile := host.JoinPath(tmpFolder, "check_config.yaml")
	_, err = host.WriteFile(tmpCheckConfigFile, []byte(checkConfig))
	if err != nil {
		s.T().Fatalf("Failed to write check config: %v", err)
	}

	checkConfigDir := fmt.Sprintf("%s.d", checkName)
	configFile := host.JoinPath(confFolder, "conf.d", checkConfigDir, "conf.yaml")

	// Create directory if it doesn't exist and copy config
	checkConfigDirPath := host.JoinPath(confFolder, "conf.d", checkConfigDir)
	if s.descriptor.Family() == e2eos.WindowsFamily {
		_, _ = host.Execute(fmt.Sprintf("if not exist \"%s\" mkdir \"%s\"", checkConfigDirPath, checkConfigDirPath))
		_, err = host.Execute(fmt.Sprintf("copy %s %s", tmpCheckConfigFile, configFile))
	} else {
		// Create directory and set ownership to dd-agent
		_, _ = host.Execute(fmt.Sprintf("sudo mkdir -p %s", checkConfigDirPath))
		_, _ = host.Execute(fmt.Sprintf("sudo chown dd-agent:dd-agent %s", checkConfigDirPath))
		_, err = host.Execute(fmt.Sprintf("sudo cp %s %s", tmpCheckConfigFile, configFile))
		if err == nil {
			_, _ = host.Execute(fmt.Sprintf("sudo chown dd-agent:dd-agent %s", configFile))
		}
	}
	if err != nil {
		s.T().Fatalf("Failed to copy check config: %v", err)
	}

	// Run the check and parse JSON output
	output, err := host.Execute(fmt.Sprintf("sudo datadog-agent check %s --json --extracfgpath %s", checkName, extraConfigFilePath))
	if err != nil {
		s.T().Logf("Check %s failed to execute: %v", checkName, err)
		return false
	}

	// Parse the JSON output and check the Runner.TotalRuns field
	data := check.ParseJSONOutput(s.T(), []byte(output))
	if len(data) == 0 {
		s.T().Logf("Check %s produced no output data", checkName)
		return false
	}

	// Check if the check actually ran by inspecting TotalRuns
	runner := data[0].Runner
	if runner.TotalRuns == 0 {
		s.T().Logf("Check %s did not run (TotalRuns=0, TotalErrors=%d, TotalWarnings=%d)",
			checkName, runner.TotalErrors, runner.TotalWarnings)
		return false
	}

	// Log success with runner statistics
	s.T().Logf("Check %s ran successfully (TotalRuns=%d, TotalErrors=%d, TotalWarnings=%d)",
		checkName, runner.TotalRuns, runner.TotalErrors, runner.TotalWarnings)
	return true
}

// ============================================================================
// Test Functions
// ============================================================================

// assertExcludedChecksAreBlocked verifies that excluded checks are blocked in basic mode.
// Tests both scheduler behavior (via status API) and CLI behavior.
func (s *infraBasicSuite) assertExcludedChecksAreBlocked() { //nolint:unused
	// List of integrations that should NOT run in basic mode
	excludedChecks := []string{
		"container_lifecycle",
		"container_image",
		"container",
		"kubelet",
		"docker",
		"orchestrator_pod",
		"cri",
		"containerd",
		"coredns",
		"kubernetes_apiserver",
		"datadog_cluster_agent",
		"kube_apiserver_metrics",
	}

	agentConfig := `
api_key: "00000000000000000000000000000000"
site: "datadoghq.com"
infrastructure_mode: "basic"
logs_enabled: false
apm_config:
  enabled: false
process_config:
  enabled: false
telemetry:
  enabled: true
`

	checkConfig := `
init_config:
instances:
  - {}
`

	s.T().Run("via_status_api", func(t *testing.T) {
		// First, verify checks are NOT scheduled by querying running agent
		stats, err := s.getRunnerStats()
		require.NoError(t, err, "Agent must be running for status API test")

		for _, checkName := range excludedChecks {
			scheduled := s.isCheckScheduled(checkName, stats)
			assert.False(t, scheduled, "Check %s should NOT be scheduled in basic mode", checkName)
		}
	})

	s.T().Run("via_cli", func(t *testing.T) {
		// Then verify checks are blocked via CLI
		for _, checkName := range excludedChecks {
			ran := s.verifyCheckRunsWithConfig(checkName, checkConfig, agentConfig)
			assert.False(t, ran, "Check %s should be blocked via CLI in basic mode", checkName)
		}
	})
}

// assertAllowedChecksWork verifies that allowed checks work in basic mode.
// Tests both scheduler behavior (via status API) and CLI behavior.
func (s *infraBasicSuite) assertAllowedChecksWork() { //nolint:unused
	// List of checks from the infra basic allowlist
	allowedChecks := []string{
		"cpu",
		"memory",
		"disk",
		"uptime",
		"load",
		"network",
		"ntp",
		"io",
		"file_handle",
		"system_core",
		"telemetry",
	}

	agentConfig := `
api_key: "00000000000000000000000000000000"
site: "datadoghq.com"
infrastructure_mode: "basic"
logs_enabled: false
apm_config:
  enabled: false
process_config:
  enabled: false
telemetry:
  enabled: true
`

	checkConfig := `
init_config:
instances:
  - {}
`

	s.T().Run("via_status_api", func(t *testing.T) {
		// First, verify checks are actually scheduled by querying running agent
		stats, err := s.getRunnerStats()
		require.NoError(t, err, "Agent must be running for status API test")

		for _, checkName := range allowedChecks {
			scheduled := s.isCheckScheduled(checkName, stats)
			assert.True(t, scheduled, "Check %s should be scheduled and running in basic mode", checkName)
		}
	})

	s.T().Run("via_cli", func(t *testing.T) {
		// Then verify checks can be run via CLI
		for _, checkName := range allowedChecks {
			ran := s.verifyCheckRunsWithConfig(checkName, checkConfig, agentConfig)
			assert.True(t, ran, "Check %s must be runnable via CLI in basic mode", checkName)
		}
	})
}
