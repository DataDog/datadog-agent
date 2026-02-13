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

		// Create a remote host
		remoteHost, err := ec2.NewVM(awsEnv, "dockervm")
		if err != nil {
			return err
		}
		err = remoteHost.Export(ctx, &env.RemoteHost.HostOutput)
		if err != nil {
			return err
		}

		// Create a fakeintake instance on ECS Fargate
		// Skip forwarding to dddev, agenthealth is only on staging
		fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, "", fakeintake.WithoutDDDevForwarding())
		if err != nil {
			return err
		}
		err = fakeIntake.Export(ctx, &env.Fakeintake.FakeintakeOutput)
		if err != nil {
			return err
		}

		// Create a docker manager
		dockerManager, err := docker.NewManager(&awsEnv, remoteHost)
		if err != nil {
			return err
		}
		err = dockerManager.Export(ctx, &env.Docker.ManagerOutput)
		if err != nil {
			return err
		}

		// Deploy busybox containers using Docker Compose
		// These will run without proper permissions to trigger the docker permission issue
		composeBusyboxCmd, err := dockerManager.ComposeStrUp("busybox", []docker.ComposeInlineManifest{
			{
				Name:    "busybox",
				Content: pulumi.String(busyboxComposeContent),
			},
		}, pulumi.StringMap{})
		if err != nil {
			return err
		}

		// Install the agent on the remote host
		// Agent depends on containers being deployed first
		hostAgent, err := agent.NewHostAgent(&awsEnv, remoteHost,
			agentparams.WithFakeintake(fakeIntake),
			agentparams.WithAgentConfig(dockerPermissionAgentConfig),
			agentparams.WithPulumiResourceOptions(pulumi.DependsOn([]pulumi.Resource{composeBusyboxCmd})),
		)
		if err != nil {
			return err
		}
		err = hostAgent.Export(ctx, &env.Agent.HostAgentOutput)
		if err != nil {
			return err
		}

		return nil
	}
}

// Ensure dockerPermissionEnv implements the Diagnosable interface
var _ common.Diagnosable = (*dockerPermissionEnv)(nil)

// Diagnose returns diagnostic information about the environment
func (e *dockerPermissionEnv) Diagnose(outputDir string) (string, error) {
	diagnoses := []string{}

	if e.RemoteHost == nil {
		return "", errors.New("RemoteHost component is not initialized")
	}

	// Add Agent diagnose
	if e.Agent != nil {
		diagnoses = append(diagnoses, "==== Agent ====")
		dstPath, err := e.generateAndDownloadAgentFlare(outputDir)
		if err != nil {
			diagnoses = append(diagnoses, fmt.Sprintf("Failed to generate agent flare: %v", err))
		} else {
			diagnoses = append(diagnoses, "Flare archive downloaded to "+dstPath)
		}
		diagnoses = append(diagnoses, "")
	}

	// Add Docker diagnose
	if e.Docker != nil {
		diagnoses = append(diagnoses, "==== Docker ====")
		dockerDiag, err := e.diagnoseDocker()
		if err != nil {
			diagnoses = append(diagnoses, fmt.Sprintf("Failed to collect Docker diagnostics: %v", err))
		} else {
			diagnoses = append(diagnoses, dockerDiag)
		}
		diagnoses = append(diagnoses, "")
	}

	return strings.Join(diagnoses, "\n"), nil
}

func (e *dockerPermissionEnv) generateAndDownloadAgentFlare(outputDir string) (string, error) {
	if e.Agent == nil || e.RemoteHost == nil {
		return "", errors.New("Agent or RemoteHost component is not initialized")
	}

	// Generate a flare
	flareCommandOutput, err := e.Agent.Client.FlareWithError(agentclient.WithArgs([]string{"--email", "e2e-tests@datadog-agent", "--send"}))

	lines := []string{flareCommandOutput}
	if err != nil {
		lines = append(lines, err.Error())
	}
	flareCommandOutput = strings.Join(lines, "\n")

	// Find <path to flare>.zip in flare command output
	re := regexp.MustCompile(`(?m)^(.+\.zip) is going to be uploaded to Datadog$`)
	matches := re.FindStringSubmatch(flareCommandOutput)
	if len(matches) < 2 {
		return "", fmt.Errorf("output does not contain the path to the flare archive, output: %s", flareCommandOutput)
	}

	flarePath := matches[1]
	flareFileInfo, err := e.RemoteHost.Lstat(flarePath)
	if err != nil {
		return "", fmt.Errorf("failed to stat flare archive: %w", err)
	}

	dstPath := filepath.Join(outputDir, flareFileInfo.Name())

	err = e.RemoteHost.EnsureFileIsReadable(flarePath)
	if err != nil {
		return "", fmt.Errorf("failed to ensure flare archive is readable: %w", err)
	}

	err = e.RemoteHost.GetFile(flarePath, dstPath)
	if err != nil {
		return "", fmt.Errorf("failed to download flare archive: %w", err)
	}

	return dstPath, nil
}

func (e *dockerPermissionEnv) diagnoseDocker() (string, error) {
	var diag strings.Builder

	// Check Docker containers
	output, err := e.RemoteHost.Execute("docker ps -a")
	if err != nil {
		return "", fmt.Errorf("failed to list Docker containers: %w", err)
	}
	diag.WriteString("Docker containers:\n")
	diag.WriteString(output)
	diag.WriteString("\n\n")

	// Check Docker socket permissions
	output, err = e.RemoteHost.Execute("ls -l /var/run/docker.sock")
	if err != nil {
		diag.WriteString(fmt.Sprintf("Failed to check Docker socket permissions: %v\n", err))
	} else {
		diag.WriteString("Docker socket permissions:\n")
		diag.WriteString(output)
		diag.WriteString("\n\n")
	}

	// Check dd-agent user groups
	output, err = e.RemoteHost.Execute("groups dd-agent")
	if err != nil {
		diag.WriteString(fmt.Sprintf("Failed to check dd-agent groups: %v\n", err))
	} else {
		diag.WriteString("dd-agent user groups:\n")
		diag.WriteString(output)
		diag.WriteString("\n\n")
	}

	// Get logs from spam containers
	for i := 1; i <= 5; i++ {
		containerName := fmt.Sprintf("spam%d", i)
		output, err = e.RemoteHost.Execute("docker logs --tail 20 " + containerName)
		if err != nil {
			diag.WriteString(fmt.Sprintf("Failed to get logs from %s: %v\n", containerName, err))
		} else {
			diag.WriteString(fmt.Sprintf("Logs from %s (last 20 lines):\n", containerName))
			diag.WriteString(output)
			diag.WriteString("\n\n")
		}
	}

	return diag.String(), nil
}
