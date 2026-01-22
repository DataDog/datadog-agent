// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package infra provides e2e tests for infrastructure mode functionality
package infra

import (
	_ "embed"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

//go:embed fixtures/custom_mycheck.py
var customCheckPython []byte

var (
	// allowedChecks lists all checks that should work in basic mode
	allowedChecks = []string{
		"cpu",
		"memory",
		"disk",
		"uptime",
		"load", // linux only
		"network",
		"ntp",
		"io",
		"file_handle",
		"system_core",
		"telemetry",
	}

	// excludedChecks lists integrations that should NOT run in basic mode
	excludedChecks = []string{
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
)

// ============================================================================
// Type Definitions
// ============================================================================

type basicSuite struct {
	e2e.BaseSuite[environments.Host]
	descriptor e2eos.Descriptor
}

// ============================================================================
// Utility Functions
// ============================================================================

// getAllowedChecks returns the list of checks that should work on the current OS
func (s *basicSuite) getAllowedChecks() []string {
	checks := make([]string, 0, len(allowedChecks))
	for _, checkName := range allowedChecks {
		// Skip "load" check on Windows as it's Linux-only
		if checkName == "load" && s.descriptor.Family() == e2eos.WindowsFamily {
			continue
		}
		checks = append(checks, checkName)
	}
	return checks
}

func (s *basicSuite) getSuiteOptions() []e2e.SuiteOption {
	// Agent configuration for basic mode testing
	basicModeAgentConfig := `
infrastructure_mode: "basic"
logs_enabled: false
apm_config:
  enabled: false
process_config:
  enabled: false
telemetry:
  enabled: true
`

	// Minimal check configuration for integration configs
	minimalCheckConfig := `
init_config:
instances:
  - {}
`

	// Build agent options with basic mode configuration and check configurations
	agentOptions := []agentparams.Option{
		agentparams.WithAgentConfig(basicModeAgentConfig),
	}

	// Add integration configs for all allowed checks (filtered by OS)
	for _, checkName := range s.getAllowedChecks() {
		agentOptions = append(agentOptions, agentparams.WithIntegration(checkName+".d", minimalCheckConfig))
	}

	// Add integration configs for excluded checks too (to test they're blocked)
	for _, checkName := range excludedChecks {
		agentOptions = append(agentOptions, agentparams.WithIntegration(checkName+".d", minimalCheckConfig))
	}

	suiteOptions := []e2e.SuiteOption{}
	suiteOptions = append(suiteOptions, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(s.descriptor), ec2.WithInstanceType("t3.micro")),
				ec2.WithAgentOptions(agentOptions...),
			),
		),
	))

	return suiteOptions
}

// ============================================================================
// Test Functions
// ============================================================================

// TestCheckSchedulingBehavior verifies that checks are correctly scheduled or blocked in basic mode.
// Tests both scheduler behavior (via status API) and CLI behavior for allowed and excluded checks.
// Note: Check configurations are provisioned during suite setup via agentparams.WithIntegration()
func (s *basicSuite) TestCheckSchedulingBehavior() {
	// First test: Verify scheduler behavior via status API
	s.T().Run("via_status_api", func(t *testing.T) {
		t.Run("allowed_checks_scheduled", func(t *testing.T) {
			allowedChecksForOS := s.getAllowedChecks()
			t.Logf("Verifying %d allowed checks are scheduled via status API...", len(allowedChecksForOS))

			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				verifyCheckSchedulingViaStatusAPI(t, c, s.Env(), allowedChecksForOS, true)
			}, 1*time.Minute, 10*time.Second, "All allowed checks should be scheduled within 1 minute")
		})

		t.Run("excluded_checks_not_scheduled", func(t *testing.T) {
			t.Logf("Verifying %d excluded checks are not scheduled via status API...", len(excludedChecks))

			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				verifyCheckSchedulingViaStatusAPI(t, c, s.Env(), excludedChecks, false)
			}, 1*time.Minute, 10*time.Second, "All excluded checks should remain not scheduled within 1 minute")
		})
	})

	// Second test: Verify CLI behavior
	s.T().Run("via_cli", func(t *testing.T) {
		t.Run("allowed_checks_runnable", func(t *testing.T) {
			allowedChecksForOS := s.getAllowedChecks()
			t.Logf("Testing %d allowed checks via CLI...", len(allowedChecksForOS))
			for _, checkName := range allowedChecksForOS {
				ran := verifyCheckRuns(t, s.Env(), checkName)
				assert.True(t, ran, "Check %s must be runnable via CLI in basic mode", checkName)
			}
		})

		t.Run("excluded_checks_blocked", func(t *testing.T) {
			t.Logf("Testing %d excluded checks via CLI...", len(excludedChecks))
			for _, checkName := range excludedChecks {
				ran := verifyCheckRuns(t, s.Env(), checkName)
				assert.False(t, ran, "Check %s should be blocked via CLI in basic mode", checkName)
			}
		})
	})
}

// TestAdditionalCheckWorks verifies that checks can be added via infra_basic_additional_checks
// and that the hardcoded custom_ prefix pattern works.
func (s *basicSuite) TestAdditionalCheckWorks() {
	// HTTP check configuration
	httpCheckConfig := `
init_config:

instances:
  - name: Example website
    url: http://example.com
    timeout: 1
`

	// Custom check configuration (tests hardcoded custom_ prefix)
	customCheckConfig := `
init_config:
instances:
  - {}
`

	// Agent configuration with additional checks (exact name matching)
	agentConfigWithAdditionalCheck := `
infrastructure_mode: "basic"
logs_enabled: false
apm_config:
  enabled: false
process_config:
  enabled: false
telemetry:
  enabled: true
allowed_additional_checks:
  - http_check
`

	s.T().Logf("Updating environment to test additional checks and custom_ prefix")

	// Determine the correct path for the custom check Python file based on OS
	customCheckPath := "/etc/datadog-agent/checks.d/custom_mycheck.py"
	if s.descriptor.Family() == e2eos.WindowsFamily {
		customCheckPath = "C:/ProgramData/Datadog/checks.d/custom_mycheck.py"
	}

	// Update the environment with the new agent config and check integrations
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithEC2InstanceOptions(ec2.WithOS(s.descriptor), ec2.WithInstanceType("t3.micro")),
			ec2.WithAgentOptions(
				agentparams.WithAgentConfig(agentConfigWithAdditionalCheck),
				agentparams.WithIntegration("http_check.d", httpCheckConfig),
				agentparams.WithIntegration("custom_mycheck.d", customCheckConfig),
				agentparams.WithFile(customCheckPath, string(customCheckPython), true),
			),
		),
	))

	s.T().Run("additional_check_in_config", func(t *testing.T) {
		t.Run("via_status_api", func(t *testing.T) {
			t.Logf("Verifying http_check is allowed via infra_basic_additional_checks...")

			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				verifyCheckSchedulingViaStatusAPI(t, c, s.Env(), []string{"http_check"}, true)
			}, 1*time.Minute, 10*time.Second, "http_check should be allowed via infra_basic_additional_checks")
		})

		t.Run("via_cli", func(t *testing.T) {
			t.Logf("Testing http_check is allowed via infra_basic_additional_checks...")
			ran := verifyCheckRuns(t, s.Env(), "http_check")
			assert.True(t, ran, "http_check must be allowed via infra_basic_additional_checks")
		})
	})

	s.T().Run("hardcoded_custom_prefix", func(t *testing.T) {
		t.Run("via_status_api", func(t *testing.T) {
			t.Logf("Verifying custom_mycheck is allowed via hardcoded custom_ prefix...")

			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				verifyCheckSchedulingViaStatusAPI(t, c, s.Env(), []string{"custom_mycheck"}, true)
			}, 1*time.Minute, 10*time.Second, "custom_mycheck should be allowed via hardcoded custom_ prefix")
		})

		t.Run("via_cli", func(t *testing.T) {
			t.Logf("Testing custom_mycheck is allowed via hardcoded custom_ prefix...")
			ran := verifyCheckRuns(t, s.Env(), "custom_mycheck")
			assert.True(t, ran, "custom_mycheck must be allowed via hardcoded custom_ prefix")
		})
	})
}
