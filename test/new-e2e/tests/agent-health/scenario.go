// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

import (
	_ "embed"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/registry"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
)

//go:embed fixtures/agent_config.yaml
var healthPlatformAgentConfig string

//go:embed fixtures/broken_check_conf.yaml
var brokenCheckConf string

//go:embed fixtures/broken_check.py
var brokenCheckPy string

//go:embed fixtures/invalid_agent_config.yaml
var invalidAgentConfig string

func init() {
	registry.RegisterScenario("aws/agent-health-demo", agentHealthDemoRun)
}

// agentHealthDemoRun provisions an agent-health demo environment.
// It dispatches to a scenario-specific provisioner based on the demolab:scenario Pulumi config key.
//
// Pulumi config keys read by this function:
//   - demolab:demoScenario  — name of the scenario being demonstrated:
//     "docker-permissions" → Docker + busybox; issue triggered via SSH after provisioning
//     "check-failure"     → broken_check pre-deployed; issue fires immediately on first collection cycle
//     "invalid-config"    → bad check_runners value in datadog.yaml; issue fires on agent startup
//     default             → plain EC2 VM with health_platform enabled, no issue pre-triggered
//
// Pulumi config keys read by aws.NewEnvironment (namespace "ddagent"):
//   - apiKey       — Datadog API key (required, secret)
//   - site         — Datadog site, e.g. "datad0g.com" (optional)
//   - pipelineId   — Agent CI pipeline ID (optional)
//   - agentVersion — Explicit agent version, e.g. "7.57.0" (optional)
func agentHealthDemoRun(ctx *pulumi.Context) error {
	switch config.New(ctx, "demolab").Get("demoScenario") {
	case "docker-permissions":
		return dockerPermissionEnvProvisioner()(ctx, nil)
	case "check-failure":
		return ec2.VMRunWithParams(ctx, ec2.GetParams(
			ec2.WithAgentOptions(
				agentparams.WithAgentConfig(healthPlatformAgentConfig),
				agentparams.WithIntegration("broken_check.d", brokenCheckConf),
				agentparams.WithFile(
					"/etc/datadog-agent/checks.d/broken_check.py",
					brokenCheckPy,
					true,
				),
			),
		))
	case "invalid-config":
		return ec2.VMRunWithParams(ctx, ec2.GetParams(
			ec2.WithAgentOptions(
				agentparams.WithAgentConfig(invalidAgentConfig),
			),
		))
	default:
		return ec2.VMRunWithParams(ctx, ec2.GetParams(
			ec2.WithAgentOptions(
				agentparams.WithAgentConfig(healthPlatformAgentConfig),
			),
		))
	}
}
