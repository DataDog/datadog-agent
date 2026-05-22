// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package agenthealth contains E2E tests for the agent health reporting functionality.
package agenthealth

import (
	_ "embed"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

//go:embed fixtures/docker_permission_agent_config.yaml
var dockerPermissionAgentConfig string

//go:embed fixtures/docker-compose.busybox.yaml
var busyboxComposeContent string

// ============================================================================
// Shared environment shape
// ============================================================================

// baseEC2Env captures the components common to all single-host health platform
// tests. Tests that need additional components (e.g. Docker) embed this or use
// a flat struct and implement a similar Diagnose method.
type baseEC2Env struct {
	RemoteHost *components.RemoteHost
	Agent      *components.RemoteHostAgent
	Fakeintake *components.FakeIntake
}

// newBaseEC2Env is a helper that provisions an EC2 host, a FakeIntake instance,
// and an agent with the provided agentparams options. It populates the three
// fields in env and returns any Pulumi error.
func newBaseEC2Env(
	ctx *pulumi.Context,
	env *baseEC2Env,
	vmName string,
	agentOptions ...func(*agentparams.Params) error,
) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	remoteHost, err := ec2.NewVM(awsEnv, vmName)
	if err != nil {
		return err
	}
	if err = remoteHost.Export(ctx, &env.RemoteHost.HostOutput); err != nil {
		return err
	}

	fi, err := fakeintake.NewECSFargateInstance(awsEnv, "", fakeintake.WithoutDDDevForwarding())
	if err != nil {
		return err
	}
	if err = fi.Export(ctx, &env.Fakeintake.FakeintakeOutput); err != nil {
		return err
	}

	hostAgent, err := agent.NewHostAgent(&awsEnv, remoteHost,
		append([]func(*agentparams.Params) error{agentparams.WithFakeintake(fi)}, agentOptions...)...,
	)
	if err != nil {
		return err
	}
	return hostAgent.Export(ctx, &env.Agent.HostAgentOutput)
}

// ============================================================================
// Docker Permission environment
// ============================================================================

type dockerPermissionEnv struct {
	RemoteHost *components.RemoteHost
	Agent      *components.RemoteHostAgent
	Fakeintake *components.FakeIntake
	Docker     *components.RemoteHostDocker
}

func dockerPermissionEnvProvisioner() provisioners.PulumiEnvRunFunc[dockerPermissionEnv] {
	return func(ctx *pulumi.Context, env *dockerPermissionEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		remoteHost, err := ec2.NewVM(awsEnv, "dockervm")
		if err != nil {
			return err
		}
		if err = remoteHost.Export(ctx, &env.RemoteHost.HostOutput); err != nil {
			return err
		}

		fi, err := fakeintake.NewECSFargateInstance(awsEnv, "", fakeintake.WithoutDDDevForwarding())
		if err != nil {
			return err
		}
		if err = fi.Export(ctx, &env.Fakeintake.FakeintakeOutput); err != nil {
			return err
		}

		dockerManager, err := docker.NewAWSManager(&awsEnv, remoteHost)
		if err != nil {
			return err
		}
		if err = dockerManager.Export(ctx, &env.Docker.ManagerOutput); err != nil {
			return err
		}

		composeBusyboxCmd, err := dockerManager.ComposeStrUp("busybox", []docker.ComposeInlineManifest{
			{Name: "busybox", Content: pulumi.String(busyboxComposeContent)},
		}, pulumi.StringMap{})
		if err != nil {
			return err
		}

		hostAgent, err := agent.NewHostAgent(&awsEnv, remoteHost,
			agentparams.WithFakeintake(fi),
			agentparams.WithAgentConfig(dockerPermissionAgentConfig),
			agentparams.WithPulumiResourceOptions(pulumi.DependsOn([]pulumi.Resource{composeBusyboxCmd})),
		)
		if err != nil {
			return err
		}
		return hostAgent.Export(ctx, &env.Agent.HostAgentOutput)
	}
}

var _ common.Diagnosable = (*dockerPermissionEnv)(nil)

func (e *dockerPermissionEnv) Diagnose(outputDir string) (string, error) {
	var parts []string
	if e.Agent != nil {
		parts = append(parts, "==== Agent ====")
		dst, err := e.generateAndDownloadAgentFlare(outputDir)
		if err != nil {
			parts = append(parts, fmt.Sprintf("flare error: %v", err))
		} else {
			parts = append(parts, "flare: "+dst)
		}
	}
	if e.Docker != nil {
		parts = append(parts, "==== Docker ====")
		diag, err := e.diagnoseDocker()
		if err != nil {
			parts = append(parts, fmt.Sprintf("docker diag error: %v", err))
		} else {
			parts = append(parts, diag)
		}
	}
	return strings.Join(parts, "\n"), nil
}

func (e *dockerPermissionEnv) generateAndDownloadAgentFlare(outputDir string) (string, error) {
	if e.Agent == nil || e.RemoteHost == nil {
		return "", errors.New("agent or host not initialized")
	}
	out, err := e.Agent.Client.FlareWithError(agentclient.WithArgs([]string{"--email", "e2e-tests@datadog-agent", "--send"}))
	allOut := out
	if err != nil {
		allOut = out + "\n" + err.Error()
	}
	re := regexp.MustCompile(`(?m)^(.+\.zip) is going to be uploaded to Datadog$`)
	m := re.FindStringSubmatch(allOut)
	if len(m) < 2 {
		return "", fmt.Errorf("no flare archive path in output: %s", allOut)
	}
	flarePath := m[1]
	info, err := e.RemoteHost.Lstat(flarePath)
	if err != nil {
		return "", fmt.Errorf("stat flare: %w", err)
	}
	dst := filepath.Join(outputDir, info.Name())
	if err = e.RemoteHost.EnsureFileIsReadable(flarePath); err != nil {
		return "", fmt.Errorf("chmod flare: %w", err)
	}
	if err = e.RemoteHost.GetFile(flarePath, dst); err != nil {
		return "", fmt.Errorf("download flare: %w", err)
	}
	return dst, nil
}

func (e *dockerPermissionEnv) diagnoseDocker() (string, error) {
	var sb strings.Builder
	cmds := []struct{ label, cmd string }{
		{"containers", "docker ps -a"},
		{"socket perms", "ls -l /var/run/docker.sock"},
		{"dd-agent groups", "groups dd-agent"},
	}
	for _, c := range cmds {
		out, err := e.RemoteHost.Execute(c.cmd)
		if err != nil {
			sb.WriteString(fmt.Sprintf("[%s] error: %v\n", c.label, err))
		} else {
			sb.WriteString(fmt.Sprintf("[%s]\n%s\n", c.label, out))
		}
	}
	return sb.String(), nil
}

// ============================================================================
// Check Failure environment
// ============================================================================

// checkFailureAgentConf is the agent config used by the check failure suite.
const checkFailureAgentConf = `
health_platform:
  enabled: true
  forwarder:
    interval: 30s
`

// brokenCheckConf is the check configuration file content (conf.d).
const brokenCheckConf = `
init_config:
instances:
  - {}
`

// brokenCheckPy is a Python check that always raises an exception.
const brokenCheckPy = `
from datadog_checks.base import AgentCheck

class BrokenCheck(AgentCheck):
    def check(self, instance):
        raise Exception("synthetic failure for e2e health platform test")
`

type checkFailureEnv struct {
	RemoteHost *components.RemoteHost
	Agent      *components.RemoteHostAgent
	Fakeintake *components.FakeIntake
}

func checkFailureEnvProvisioner() provisioners.PulumiEnvRunFunc[checkFailureEnv] {
	return func(ctx *pulumi.Context, env *checkFailureEnv) error {
		base := &baseEC2Env{
			RemoteHost: env.RemoteHost,
			Agent:      env.Agent,
			Fakeintake: env.Fakeintake,
		}
		return newBaseEC2Env(ctx, base, "checkfailurevm",
			agentparams.WithAgentConfig(checkFailureAgentConf),
			// Deploy the broken check via the integration mechanism so it is
			// present when the agent first starts.
			agentparams.WithIntegration("broken_check.d", brokenCheckConf),
			agentparams.WithFile(
				"/etc/datadog-agent/checks.d/broken_check.py",
				brokenCheckPy,
				true, // useSudo
			),
		)
	}
}

var _ common.Diagnosable = (*checkFailureEnv)(nil)

func (e *checkFailureEnv) Diagnose(outputDir string) (string, error) {
	if e.Agent == nil {
		return "", errors.New("agent not initialized")
	}
	// Run diagnose command and include health-issues output.
	out := e.Agent.Client.Diagnose(agentclient.WithArgs([]string{"--include", healthIssueSuite}))
	return "==== agent diagnose health-issues ====\n" + out, nil
}
