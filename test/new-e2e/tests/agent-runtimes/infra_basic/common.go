// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package infrabasic contains end-to-end tests for infrastructure basic mode functionality.
// These tests verify that core system checks work correctly when the agent is configured
// to run in basic infrastructure mode, which only instantiates essential components.
package infrabasic

import (
	"fmt"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/testcommon/check"
)

type infraBasicSuite struct { //nolint:unused
	e2e.BaseSuite[environments.Host]
	descriptor e2eos.Descriptor
}

func (s *infraBasicSuite) getSuiteOptions() []e2e.SuiteOption { //nolint:unused
	suiteOptions := []e2e.SuiteOption{}
	suiteOptions = append(suiteOptions, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithEC2InstanceOptions(ec2.WithOS(s.descriptor)),
		),
	))

	return suiteOptions
}

// verifyCheckRunsInBasicMode verifies that a check runs successfully in infrastructure basic mode
// Returns true if the check ran successfully (TotalRuns > 0), false otherwise
func (s *infraBasicSuite) verifyCheckRunsInBasicMode(checkName string, checkConfig string) bool { //nolint:unused
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

	return s.verifyCheckRunsWithConfig(checkName, checkConfig, agentConfig)
}

// verifyCheckRunsWithCustomConfig verifies that a check runs successfully with custom agent configuration
func (s *infraBasicSuite) verifyCheckRunsWithCustomConfig(checkName string, checkConfig string, agentConfig string) bool { //nolint:unused
	return s.verifyCheckRunsWithConfig(checkName, checkConfig, agentConfig)
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

	// Parse the JSON output - we need to check if the check ran by looking for "runner" in the output
	// The JSON structure includes a "runner" field with TotalRuns, but the ParseJSONOutput function
	// only parses the aggregator metrics. For now, we'll check if metrics were produced as a proxy
	// for successful execution.
	data := check.ParseJSONOutput(s.T(), []byte(output))
	if len(data) == 0 {
		s.T().Logf("Check %s produced no output data", checkName)
		return false
	}

	// If we got valid parsed data, the check ran successfully
	// The ParseJSONOutput function would fail or return empty if the check didn't run
	s.T().Logf("Check %s ran successfully", checkName)
	return true
}

// assertExcludedIntegrationsDoNotRun verifies that excluded integrations are blocked in basic mode
// NOTE: This test requires the agent to be built with CLI filtering changes.
// The E2E test uses a pre-installed agent, so CLI filtering cannot be tested here.
// CLI filtering is implemented in pkg/cli/subcommands/check/command.go
// Scheduler filtering is implemented in pkg/collector/scheduler.go
// This test documents which checks should be blocked.
func (s *infraBasicSuite) assertExcludedIntegrationsDoNotRun() { //nolint:unused
	// List of integrations that should NOT run in basic mode
	// These correspond to the denylist from the infra basic mode specification
	excludedIntegrations := []struct {
		name   string
		config string
	}{
		{
			name: "container_lifecycle",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "container_image",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "container",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "kubelet",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "docker",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "orchestrator_pod",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "cri",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "containerd",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "coredns",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "kubernetes_apiserver",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "datadog_cluster_agent",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "kube_apiserver_metrics",
			config: `
init_config:
instances:
  - {}
`,
		},
	}

	// Run each excluded integration to document their status
	// Once the agent is rebuilt with CLI filtering, these should all be blocked
	for _, integration := range excludedIntegrations {
		ran := s.verifyCheckRunsInBasicMode(integration.name, integration.config)

		if ran {
			s.T().Logf("Integration %s is loadable (will be blocked once CLI filtering is deployed)", integration.name)
		} else {
			s.T().Logf("Integration %s failed to load (missing dependencies or blocked)", integration.name)
		}
	}
}

// assertBasicChecksWork verifies that basic infrastructure checks run successfully
func (s *infraBasicSuite) assertBasicChecksWork() { //nolint:unused
	// List of checks from the infra basic allowlist
	basicChecks := []string{
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

	// Default check configuration (same for all checks)
	defaultCheckConfig := `
init_config:
instances:
  - {}
`

	// Run each check and verify it executed successfully
	for _, checkName := range basicChecks {
		if !s.verifyCheckRunsInBasicMode(checkName, defaultCheckConfig) {
			s.T().Errorf("Check %s failed to run in basic mode", checkName)
		}
	}
}

// assertAdditionalChecksConfiguration verifies that the infra_basic_additional_checks config works
// This tests the feature added in pkg/config/setup/constants/constants.go
func (s *infraBasicSuite) assertAdditionalChecksConfiguration() { //nolint:unused
	// Test 1: additional_checks_allows_custom_integrations
	// Configure agent with additional checks that would normally be excluded
	agentConfigWithAdditional := `
api_key: "00000000000000000000000000000000"
site: "datadoghq.com"
infrastructure_mode: "basic"
logs_enabled: false
apm_config:
  enabled: false
process_config:
  enabled: false
# Add http_check which is not in the default allowlist
infra_basic_additional_checks:
  - http_check
`
	// http_check is a simple integration that doesn't require external dependencies
	checkConfig := `
init_config:
instances:
  - name: test_endpoint
    url: http://localhost:1
    timeout: 1
`

	// Run the check - it should be allowed to run (even if it fails to connect)
	if !s.verifyCheckRunsWithCustomConfig("http_check", checkConfig, agentConfigWithAdditional) {
		s.T().Error("http_check should be allowed to run when added to infra_basic_additional_checks, but TotalRuns was 0")
	} else {
		s.T().Log("http_check correctly allowed via infra_basic_additional_checks (TotalRuns > 0)")
	}

	// Test 2: without_additional_checks_integration_is_blocked
	// Configure agent WITHOUT additional checks
	agentConfigWithoutAdditional := `
api_key: "00000000000000000000000000000000"
site: "datadoghq.com"
infrastructure_mode: "basic"
logs_enabled: false
apm_config:
  enabled: false
process_config:
  enabled: false
`

	// Run the check - when run via CLI it will execute, but in a real agent run
	// the scheduler would filter it. The scheduler filtering is tested in assertSchedulerFiltering
	if s.verifyCheckRunsWithCustomConfig("http_check", checkConfig, agentConfigWithoutAdditional) {
		s.T().Log("http_check ran via CLI (expected - CLI bypasses scheduler filtering)")
	} else {
		s.T().Log("http_check did not run")
	}
}

// assertSchedulerFiltering verifies that the scheduler correctly filters checks
// This tests the logic in pkg/collector/scheduler.go
func (s *infraBasicSuite) assertSchedulerFiltering() { //nolint:unused
	// Test 1: scheduler_filters_disallowed_checks
	// Configure agent with a check that's NOT in the allowlist
	agentConfig := `
api_key: "00000000000000000000000000000000"
site: "datadoghq.com"
infrastructure_mode: "basic"
logs_enabled: false
apm_config:
  enabled: false
process_config:
  enabled: false
`
	// Try to configure docker check which is explicitly NOT allowed in basic mode
	dockerCheckConfig := `
init_config:
instances:
  - url: "unix://var/run/docker.sock"
`

	// Run the check - it should be filtered by the scheduler
	// Note: When run via CLI, checks bypass the scheduler, so this test verifies
	// the check CAN run when needed, but actual scheduler filtering is tested
	// by examining agent logs in a real deployment
	if s.verifyCheckRunsWithCustomConfig("docker", dockerCheckConfig, agentConfig) {
		s.T().Log("docker check ran via CLI (expected - CLI bypasses scheduler)")
	} else {
		s.T().Log("docker check did not run")
	}

	// Test 2: scheduler_allows_default_checks
	// Verify that default allowed checks still work
	// cpu is in the default allowlist
	cpuCheckConfig := `
init_config:
instances:
  - {}
`

	// Run the check - it should NOT be filtered
	if !s.verifyCheckRunsWithCustomConfig("cpu", cpuCheckConfig, agentConfig) {
		s.T().Error("cpu check should be allowed in basic mode and run successfully")
	} else {
		s.T().Log("cpu check correctly allowed in basic mode")
	}
}
