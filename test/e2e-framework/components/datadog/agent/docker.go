// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agent

import (
	"fmt"
	"maps"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	remoteComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
)

const (
	agentContainerName = "datadog-agent"
)

type DockerAgentOutput struct {
	components.JSONImporter

	DockerManager docker.ManagerOutput `json:"dockerManager"`
	ContainerName string               `json:"containerName"`
	FIPSEnabled   bool                 `json:"fipsEnabled"`
}

// DockerAgent is a Docker installer on a remote Host
type DockerAgent struct {
	pulumi.ResourceState
	components.Component

	DockerManager *docker.Manager     `pulumi:"dockerManager"`
	ContainerName pulumi.StringOutput `pulumi:"containerName"`
	FIPSEnabled   pulumi.BoolOutput   `pulumi:"fipsEnabled"`
}

func (h *DockerAgent) Export(ctx *pulumi.Context, out *DockerAgentOutput) error {
	return components.Export(ctx, h, out)
}

func NewDockerAgent(e config.Env, vm *remoteComp.Host, manager *docker.Manager, options ...dockeragentparams.Option) (*DockerAgent, error) {
	return components.NewComponent(e, vm.Name(), func(comp *DockerAgent) error {
		params, err := dockeragentparams.NewParams(e, options...)
		if err != nil {
			return err
		}

		comp.FIPSEnabled = pulumi.Bool(e.AgentFIPS() || params.FIPS).ToBoolOutput()
		fullImagePath := params.FullImagePath
		if fullImagePath == "" {
			fullImagePath = dockerAgentFullImagePath(e, params.Repository, params.ImageTag, false, params.FIPS, params.JMX, params.WindowsImage)
		}

		// Check FullImagePath exists in internal registry
		exists, err := e.InternalRegistryFullImagePathExists(fullImagePath)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("image %q not found in the internal registry", fullImagePath)
		}

		// We can have multiple compose files in compose.
		composeContents := []docker.ComposeInlineManifest{dockerAgentComposeManifest(fullImagePath, e.AgentAPIKey(), params.AgentServiceEnvironment)}
		composeContents = append(composeContents, params.ExtraComposeManifests...)

		opts := make([]pulumi.ResourceOption, 0, len(params.PulumiDependsOn)+1)
		opts = append(opts, params.PulumiDependsOn...)
		opts = append(opts, pulumi.Parent(comp))
		_, err = manager.ComposeStrUp("agent", composeContents, params.EnvironmentVariables, opts...)
		if err != nil {
			return err
		}

		// Fill component
		comp.FIPSEnabled = pulumi.Bool(params.FIPS).ToBoolOutput()
		comp.DockerManager = manager
		comp.ContainerName = pulumi.String(agentContainerName).ToStringOutput()

		return nil
	})
}

func dockerAgentComposeManifest(agentImagePath string, apiKey pulumi.StringInput, envVars pulumi.Map) docker.ComposeInlineManifest {
	runInPrivileged := false
	for k := range envVars {
		if strings.HasPrefix(k, "DD_SYSTEM_PROBE_") {
			runInPrivileged = true
			break
		}
	}

	agentManifestContent := pulumi.All(apiKey, envVars).ApplyT(func(args []interface{}) (string, error) {
		apiKeyResolved := args[0].(string)
		envVarsResolved := args[1].(map[string]any)
		agentManifest := docker.ComposeManifest{
			Version: "3.9",
			Services: map[string]docker.ComposeManifestService{
				"agent": {
					Privileged:    runInPrivileged,
					Image:         agentImagePath,
					ContainerName: agentContainerName,
					Volumes: []string{
						"/var/run/docker.sock:/var/run/docker.sock",
						"/proc/:/host/proc",
						"/sys/fs/cgroup/:/host/sys/fs/cgroup",
						"/var/run/datadog:/var/run/datadog",
						"/sys/kernel/tracing:/sys/kernel/tracing",
					},
					Environment: map[string]any{
						"DD_API_KEY": apiKeyResolved,
						// DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED is compatible with Agent 7.35+
						"DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED": true,
					},
					Pid:   "host",
					Ports: []string{"8125:8125/udp", "8126:8126/tcp"},
				},
			},
		}
		maps.Copy(agentManifest.Services["agent"].Environment, envVarsResolved)
		data, err := yaml.Marshal(agentManifest)
		return string(data), err
	}).(pulumi.StringOutput)

	return docker.ComposeInlineManifest{
		Name:    "agent",
		Content: agentManifestContent,
	}
}
