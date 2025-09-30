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

type infraBasicSuite struct {
	e2e.BaseSuite[environments.Host]
	descriptor e2eos.Descriptor
}

func (s *infraBasicSuite) getSuiteOptions() []e2e.SuiteOption {
	suiteOptions := []e2e.SuiteOption{}
	suiteOptions = append(suiteOptions, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithEC2InstanceOptions(ec2.WithOS(s.descriptor)),
		),
	))

	return suiteOptions
}

// runCheckInBasicMode runs a check in infrastructure basic mode
func (s *infraBasicSuite) runCheckInBasicMode(checkName string, checkConfig string) []check.Metric {
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

// assertExcludedIntegrationsDoNotRun verifies that integrations that should be excluded in basic mode do not run
func (s *infraBasicSuite) assertExcludedIntegrationsDoNotRun() {
	// List of integrations that should NOT run in basic mode
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
func (s *infraBasicSuite) assertBasicChecksWork() {
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
