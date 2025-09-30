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
	"strings"
	"testing"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/testcommon/check"
	checkUtils "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-runtimes/checks/common"
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

// runCheckInBasicMode runs a check in infrastructure basic mode
func (s *infraBasicSuite) runCheckInBasicMode(checkName string, checkConfig string) []check.Metric { //nolint:unused
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

	ctx := checkUtils.CheckContext{
		CheckName:    checkName,
		OSDescriptor: s.descriptor,
		AgentConfig:  agentConfig,
		CheckConfig:  checkConfig,
		IsNewVersion: false,
	}

	metrics := checkUtils.RunCheck(s.T(), s.Env(), ctx)
	return metrics
}

// runCheckInBasicModeWithCustomConfig runs a check with custom agent configuration
func (s *infraBasicSuite) runCheckInBasicModeWithCustomConfig(checkName string, checkConfig string, agentConfig string) []check.Metric { //nolint:unused
	ctx := checkUtils.CheckContext{
		CheckName:    checkName,
		OSDescriptor: s.descriptor,
		AgentConfig:  agentConfig,
		CheckConfig:  checkConfig,
		IsNewVersion: false,
	}

	metrics := checkUtils.RunCheck(s.T(), s.Env(), ctx)
	return metrics
}

// assertExcludedIntegrationsDoNotRun verifies that integrations that should be excluded in basic mode do not run
// These checks should be filtered out by the scheduler based on the allowlist in pkg/config/setup/constants/constants.go
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

	// Run each excluded integration and verify it produces no metrics or fails
	for _, integration := range excludedIntegrations {
		s.T().Run(fmt.Sprintf("excluded_integration_%s", integration.name), func(t *testing.T) {
			metrics := s.runCheckInBasicMode(integration.name, integration.config)

			// In basic mode, these integrations should either:
			// 1. Produce no metrics (not loaded)
			// 2. Fail to run (not available)
			// 3. Produce minimal/error metrics
			if len(metrics) == 0 {
				t.Logf("Integration %s correctly excluded from basic mode (no metrics)", integration.name)
				return
			}

			// If metrics are produced, they should be minimal or error indicators
			// Log the metrics for debugging
			t.Logf("Integration %s produced %d metrics in basic mode", integration.name, len(metrics))

			// Check if any metrics indicate the integration is not properly running
			hasErrorMetrics := false
			for _, metric := range metrics {
				if len(metric.Points) > 0 {
					// Look for error indicators in metric names or tags
					if strings.Contains(metric.Metric, "error") ||
						strings.Contains(metric.Metric, "failed") ||
						strings.Contains(metric.Metric, "unavailable") {
						hasErrorMetrics = true
						break
					}
				}
			}

			if hasErrorMetrics {
				t.Logf("Integration %s correctly shows error metrics in basic mode", integration.name)
			} else {
				t.Errorf("Integration %s should be excluded from basic mode but appears to be running normally", integration.name)
			}
		})
	}
}

// assertBasicChecksWork verifies that basic infrastructure checks work correctly
func (s *infraBasicSuite) assertBasicChecksWork() { //nolint:unused
	// List of checks that should work in basic mode
	basicChecks := []struct {
		name   string
		config string
	}{
		{
			name: "cpu",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "agent_telemetry",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "agentcrashdetect",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "disk",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "file_handle",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "filehandles",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "io",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "load",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "memory",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "network",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "ntp",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "process",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "service_discovery",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "system",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "system_core",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "system_swap",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "telemetry",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "telemetryCheck",
			config: `
init_config:
instances:
  - {}
`,
		},
		{
			name: "uptime",
			config: `
init_config:
instances:
  - {}
`,
		},
	}

	// Windows-specific checks
	if s.descriptor.Family() == e2eos.WindowsFamily {
		windowsChecks := []struct {
			name   string
			config string
		}{
			{
				name: "win32_event_log",
				config: `
init_config:
instances:
  - {}
`,
			},
			{
				name: "wincrashdetect",
				config: `
init_config:
instances:
  - {}
`,
			},
			{
				name: "winkmem",
				config: `
init_config:
instances:
  - {}
`,
			},
			{
				name: "winproc",
				config: `
init_config:
instances:
  - {}
`,
			},
		}
		basicChecks = append(basicChecks, windowsChecks...)
	}

	// Run each check and verify it produces metrics
	for _, check := range basicChecks {
		s.T().Run(fmt.Sprintf("check_%s", check.name), func(t *testing.T) {
			metrics := s.runCheckInBasicMode(check.name, check.config)

			// Verify that the check produced some metrics
			if len(metrics) == 0 {
				t.Errorf("Check %s produced no metrics in basic mode", check.name)
				return
			}

			// Log some basic info about the metrics
			t.Logf("Check %s produced %d metrics in basic mode", check.name, len(metrics))

			// Verify that at least one metric has a valid value
			hasValidMetric := false
			for _, metric := range metrics {
				if len(metric.Points) > 0 {
					hasValidMetric = true
					break
				}
			}

			if !hasValidMetric {
				t.Errorf("Check %s produced no metrics with valid values in basic mode", check.name)
			}
		})
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
	metrics := s.runCheckInBasicModeWithCustomConfig("http_check", checkConfig, agentConfigWithAdditional)

	// The check should produce metrics (even if just error metrics)
	// If the check was filtered out by the scheduler, we'd get no metrics at all
	if len(metrics) == 0 {
		s.T().Error("http_check should be allowed to run when added to infra_basic_additional_checks, but produced no metrics")
	} else {
		s.T().Logf("http_check correctly allowed via infra_basic_additional_checks, produced %d metrics", len(metrics))
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

	// Run the check - it should be blocked by the scheduler
	metricsBlocked := s.runCheckInBasicModeWithCustomConfig("http_check", checkConfig, agentConfigWithoutAdditional)

	// Without being in the additional_checks list, http_check should be filtered
	if len(metricsBlocked) > 0 {
		// Check if these are error metrics indicating the check was blocked
		hasBlockedIndicator := false
		for _, metric := range metricsBlocked {
			if strings.Contains(metric.Metric, "error") ||
				strings.Contains(metric.Metric, "skipped") {
				hasBlockedIndicator = true
				break
			}
		}

		if !hasBlockedIndicator {
			s.T().Error("http_check should be blocked in basic mode without infra_basic_additional_checks")
		}
	} else {
		s.T().Log("http_check correctly blocked in basic mode (no metrics produced)")
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
	dockerMetrics := s.runCheckInBasicModeWithCustomConfig("docker", dockerCheckConfig, agentConfig)

	// Docker check should produce no metrics because it's filtered by scheduler
	// The scheduler.go code logs: "Check %s is not allowed in infra basic mode, skipping"
	if len(dockerMetrics) > 0 {
		s.T().Errorf("docker check should be filtered by scheduler in basic mode, but produced %d metrics", len(dockerMetrics))
		s.T().Logf("Unexpected metrics from docker check:")
		for _, m := range dockerMetrics {
			s.T().Logf("  - %s: %v", m.Metric, m.Points)
		}
	} else {
		s.T().Log("docker check correctly filtered by scheduler in basic mode (no metrics produced)")
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
	cpuMetrics := s.runCheckInBasicModeWithCustomConfig("cpu", cpuCheckConfig, agentConfig)

	// CPU check should produce metrics
	if len(cpuMetrics) == 0 {
		s.T().Error("cpu check should be allowed in basic mode and produce metrics")
	} else {
		s.T().Logf("cpu check correctly allowed in basic mode, produced %d metrics", len(cpuMetrics))
	}
}
