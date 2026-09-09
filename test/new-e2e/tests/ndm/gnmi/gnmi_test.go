// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package gnmi contains e2e tests for the gNMI core check.
package gnmi

import (
	"embed"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	fakeserverSourcePath = "compose/fakeserver"
	deviceIP             = "dd-gnmi"
	interfaceName        = "eth0"
)

//go:embed compose/gnmiCompose.yaml
var gnmiCompose string

//go:embed config/gnmi.yaml
var gnmiConfig string

//go:embed config/profiles/interface-stats.yaml
var interfaceStatsProfile string

//go:embed compose/fakeserver/Dockerfile compose/fakeserver/main.go
var fakeserverSource embed.FS

// gnmiDockerProvisioner defines a stack with a docker agent on an EC2 VM,
// a compose-built gNMI fakeserver sidecar, and fakeintake.
func gnmiDockerProvisioner() provisioners.Provisioner {
	return provisioners.NewTypedPulumiProvisioner("", func(ctx *pulumi.Context, env *environments.DockerHost) error {
		name := "gnmivm"
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		host, err := ec2.NewVM(awsEnv, name)
		if err != nil {
			return err
		}
		host.Export(ctx, &env.RemoteHost.HostOutput)

		fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, name)
		if err != nil {
			return err
		}
		fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput)

		filemanager := host.OS.FileManager()

		createBuildDirCommand, buildPath, err := filemanager.TempDirectory("gnmi-fakeserver")
		if err != nil {
			return err
		}
		buildCommands, err := uploadFakeserverSource(filemanager, buildPath, createBuildDirCommand)
		if err != nil {
			return err
		}

		createConfigDirCommand, configPath, err := filemanager.TempDirectory("config")
		if err != nil {
			return err
		}

		gnmiConfigCommand, err := filemanager.CopyInlineFile(pulumi.String(gnmiConfig), path.Join(configPath, "gnmi.yaml"),
			pulumi.DependsOn([]pulumi.Resource{createConfigDirCommand}))
		if err != nil {
			return err
		}

		profileRemotePath := path.Join(configPath, "profiles", "interface-stats.yaml")
		createProfilesDirCommand, err := filemanager.CreateDirectoryForFile(profileRemotePath, false,
			pulumi.DependsOn([]pulumi.Resource{createConfigDirCommand}))
		if err != nil {
			return err
		}

		profileCommand, err := filemanager.CopyInlineFile(pulumi.String(interfaceStatsProfile), profileRemotePath,
			pulumi.DependsOn([]pulumi.Resource{createProfilesDirCommand}))
		if err != nil {
			return err
		}

		dockerManager, err := docker.NewAWSManager(&awsEnv, host)
		if err != nil {
			return err
		}
		dockerManager.Export(ctx, &env.Docker.ManagerOutput)

		envVars := pulumi.StringMap{
			"BUILD_DIR":  pulumi.String(buildPath),
			"CONFIG_DIR": pulumi.String(configPath),
		}
		composeDependencies := []pulumi.Resource{gnmiConfigCommand, profileCommand}
		composeDependencies = append(composeDependencies, buildCommands...)

		dockerAgent, err := agent.NewDockerAgent(&awsEnv, host, dockerManager,
			dockeragentparams.WithFakeintake(fakeIntake),
			dockeragentparams.WithExtraComposeManifest("gnmi", pulumi.String(gnmiCompose)),
			dockeragentparams.WithEnvironmentVariables(envVars),
			dockeragentparams.WithPulumiDependsOn(pulumi.DependsOn(composeDependencies)),
		)
		if err != nil {
			return err
		}
		dockerAgent.Export(ctx, &env.Agent.DockerAgentOutput)

		return nil
	}, nil)
}

func uploadFakeserverSource(filemanager interface {
	CopyInlineFile(content pulumi.StringInput, destinationPath string, opts ...pulumi.ResourceOption) (pulumi.Resource, error)
}, buildPath string, dependsOn pulumi.Resource) ([]pulumi.Resource, error) {
	embeddedFiles := []string{"Dockerfile", "main.go"}
	commands := []pulumi.Resource{dependsOn}

	for _, fileName := range embeddedFiles {
		fileContent, err := fakeserverSource.ReadFile(path.Join(fakeserverSourcePath, fileName))
		if err != nil {
			return nil, err
		}
		fileCommand, err := filemanager.CopyInlineFile(pulumi.String(fileContent), path.Join(buildPath, fileName),
			pulumi.DependsOn([]pulumi.Resource{dependsOn}))
		if err != nil {
			return nil, err
		}
		commands = append(commands, fileCommand)
	}

	for _, file := range []struct {
		name    string
		content string
	}{
		{name: "go.mod", content: fakeserverGoMod},
		{name: "go.sum", content: fakeserverGoSum},
	} {
		fileCommand, err := filemanager.CopyInlineFile(pulumi.String(file.content), path.Join(buildPath, file.name),
			pulumi.DependsOn([]pulumi.Resource{dependsOn}))
		if err != nil {
			return nil, err
		}
		commands = append(commands, fileCommand)
	}

	return commands, nil
}

type gnmiDockerSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

// TestGnmiSuite runs the gNMI e2e suite.
func TestGnmiSuite(t *testing.T) {
	e2e.Run(t, &gnmiDockerSuite{}, e2e.WithProvisioner(gnmiDockerProvisioner()))
}

// TestGnmiMetrics validates that the agent collects snmp.* metrics from the gNMI sidecar.
func (s *gnmiDockerSuite) TestGnmiMetrics() {
	fakeintake := s.Env().FakeIntake.Client()

	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := fakeintake.FilterMetrics("snmp.ifHCInOctets")
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics, "no snmp.ifHCInOctets metrics received yet")
		assertMetricHasTags(c, metrics, deviceIP, interfaceName)
	}, 5*time.Minute, 10*time.Second)
}

// TestGnmiTagsAreStoredOnRestart validates that collection resumes with the same tags after restart.
func (s *gnmiDockerSuite) TestGnmiTagsAreStoredOnRestart() {
	fakeintake := s.Env().FakeIntake.Client()
	var initialMetrics []*aggregator.MetricSeries
	var err error

	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		initialMetrics, err = fakeintake.FilterMetrics("snmp.ifHCInOctets")
		assert.NoError(t, err)
		assert.NotEmpty(t, initialMetrics)
	}, 5*time.Minute, 5*time.Second)

	initialTags := initialMetrics[0].Tags

	_, err = s.Env().RemoteHost.Execute("docker stop dd-gnmi")
	require.NoError(s.T(), err)

	_, err = s.Env().RemoteHost.Execute("docker start dd-gnmi")
	require.NoError(s.T(), err)

	_, err = s.Env().RemoteHost.Execute("docker restart " + s.Env().Agent.ContainerName)
	require.NoError(s.T(), err)

	err = fakeintake.FlushServerAndResetAggregators()
	require.NoError(s.T(), err)

	var metrics []*aggregator.MetricSeries
	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		metrics, err = fakeintake.FilterMetrics("snmp.ifHCInOctets")
		assert.NoError(t, err)
		assert.NotEmpty(t, metrics)
	}, 5*time.Minute, 5*time.Second)

	requireMetricHasTags(s.T(), metrics, deviceIP, interfaceName)
	require.ElementsMatch(s.T(), metrics[0].Tags, initialTags)
}
