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
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/registry"
	resAws "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"
)

//go:embed fixtures/invalid_agent_config.yaml
var invalidAgentConfig string

// demoForwarderInterval shortens the health-platform forwarder interval (default
// 15m) so demo environments surface issues within ~30s. Kept separate from the
// scenario fixtures so they stay minimal, and as a YAML file so it can't be
// broken by string escaping.
//
//go:embed fixtures/30_seconds_healthplatform_forwarder.yaml
var demoForwarderInterval string

func init() {
	registry.RegisterScenario(PulumiScenarioName, AgentHealthDemoRun)
}

// AgentHealthDemoRun provisions an agent-health demo environment.
// It dispatches to a scenario-specific provisioner based on the demolab:scenario Pulumi config key.
//
// Pulumi config keys read by this function:
//   - demolab:demoScenario  — name of the scenario being demonstrated:
//     "docker-permissions" → Docker + busybox; issue triggered via SSH after provisioning
//     "invalid-config"    → bad check_runners value in datadog.yaml; issue fires on agent startup
//     "high-memory-k8s"   → kind cluster on EC2, node Agent with a 1Gi memory limit; issue triggered via SSH
//     default             → plain EC2 VM with health_platform enabled, no issue pre-triggered
//
// Pulumi config keys read by aws.NewEnvironment (namespace "ddagent"):
//   - apiKey       — Datadog API key (required, secret)
//   - site         — Datadog site, e.g. "datad0g.com" (optional)
//   - pipelineId   — Agent CI pipeline ID (optional)
//   - agentVersion — Explicit agent version, e.g. "7.57.0" (optional)
func AgentHealthDemoRun(ctx *pulumi.Context) error {
	switch config.New(ctx, "demolab").Get("demoScenario") {
	case "docker-permissions":
		return dockerPermissionEnvProvisioner()(ctx, nil)
	case "invalid-config":
		return ec2.VMRunWithParams(ctx, ec2.GetParams(
			ec2.WithAgentOptions(
				agentparams.WithAgentConfig(invalidAgentConfig+demoForwarderInterval),
			),
		))
	case highMemoryK8sScenario:
		return highMemoryK8sRun(ctx)
	default:
		// Plain EC2 VM with the Agent installed; health_platform is enabled by
		// default, so no extra agent config is needed.
		return ec2.VMRunWithParams(ctx, ec2.GetParams())
	}
}

// highMemoryK8sRun provisions a kind cluster on an EC2 VM, with the node Agent deployed through Helm with a memory
// limit so that its usage can be pushed close to it.
func highMemoryK8sRun(ctx *pulumi.Context) error {
	awsEnv, err := resAws.NewEnvironment(ctx)
	if err != nil {
		return err
	}
	params := kindvm.ParamsFromEnvironment(awsEnv)
	if err := kindvm.WithAgentOptions(kubernetesagentparams.WithHelmValues(highMemoryK8sHelmValues))(params); err != nil {
		return err
	}
	return kindvm.RunWithEnv(ctx, awsEnv, outputs.NewKubernetes(), params)
}
