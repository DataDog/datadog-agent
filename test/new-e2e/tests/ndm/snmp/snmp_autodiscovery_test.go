// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package snmp

import (
	_ "embed"
	"fmt"
	"path"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"

	//"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/dockeragentparams"
	"github.com/DataDog/test-infra-definitions/components/docker"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed compose/autodiscoveryCompose.yaml
var autodiscoveryCompose string

//go:embed config/autodiscovery.yaml
var autodiscoveryConfig string

func snmpMultiIPDockerProvisioner() provisioners.Provisioner {
	return provisioners.NewTypedPulumiProvisioner("", func(ctx *pulumi.Context, env *environments.DockerHost) error {
		name := "snmpvm"
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		host, err := ec2.NewVM(awsEnv, name, ec2.WithOS(os.AmazonLinuxECSDefault))
		if err != nil {
			return err
		}
		host.Export(ctx, &env.RemoteHost.HostOutput)

		fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, name)
		if err != nil {
			return err
		}
		fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput)

		// Setting up SNMP-related files
		filemanager := host.OS.FileManager()
		// upload snmpsim data files
		createDataDirCommand, dataPath, err := filemanager.TempDirectory("data")
		if err != nil {
			return err
		}

		dataFiles, err := loadDataFileNames()
		if err != nil {
			return err
		}

		fileCommands := []pulumi.Resource{}
		for _, fileName := range dataFiles {
			fileContent, err := dataFolder.ReadFile(path.Join(composeDataPath, fileName))
			if err != nil {
				return err
			}
			fileCommand, err := filemanager.CopyInlineFile(pulumi.String(fileContent), path.Join(dataPath, fileName),
				pulumi.DependsOn([]pulumi.Resource{createDataDirCommand}))
			if err != nil {
				return err
			}
			fileCommands = append(fileCommands, fileCommand)
		}

		createConfigDirCommand, configPath, err := filemanager.TempDirectory("config")
		if err != nil {
			return err
		}

		configCommand, err := filemanager.CopyInlineFile(pulumi.String(autodiscoveryConfig), path.Join(configPath, "datadog.yaml"),
			pulumi.DependsOn([]pulumi.Resource{createConfigDirCommand}))
		if err != nil {
			return err
		}

		installEcrCredsHelperCmd, err := ec2.InstallECRCredentialsHelper(awsEnv, host)
		if err != nil {
			return err
		}

		dockerManager, err := docker.NewManager(&awsEnv, host, utils.PulumiDependsOn(installEcrCredsHelperCmd))
		if err != nil {
			return err
		}
		dockerManager.Export(ctx, &env.Docker.ManagerOutput)

		composeDependencies := []pulumi.Resource{createDataDirCommand, configCommand}
		composeDependencies = append(composeDependencies, fileCommands...)
		dockerAgent, err := agent.NewDockerAgent(&awsEnv, host, dockerManager,
			dockeragentparams.WithFakeintake(fakeIntake),
			dockeragentparams.WithExtraComposeManifest("snmpsim", pulumi.String(autodiscoveryCompose)),
			dockeragentparams.WithPulumiDependsOn(pulumi.DependsOn(composeDependencies)),
		)
		if err != nil {
			return err
		}
		dockerAgent.Export(ctx, &env.Agent.DockerAgentOutput)

		return nil
	}, nil)
}

type snmpAutodiscoverySuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

func TestSnmpAutodiscoverySuite(t *testing.T) {
	e2e.Run(t, &snmpAutodiscoverySuite{}, e2e.WithProvisioner(snmpMultiIPDockerProvisioner()))
}

func (s *snmpAutodiscoverySuite) TestSnmpAutodiscovery() {
	agent := s.Env().Agent
	fakeIntake := s.Env().FakeIntake.Client()
	remoteHost := s.Env().RemoteHost

	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(
		"docker exec %s bash -c 'apt-get update && apt-get install -y snmp'",
		agent.ContainerName))
	if err != nil {
		s.T().Fatalf("Failed to install snmp utility: %v", err)
	}

	snmpResult, err := remoteHost.Execute(fmt.Sprintf(
		"docker exec %s snmpwalk -v 2c -c ciscso-nexus 192.168.100.2:1161 1.3.6.1.2.1.1.3.0",
		agent.ContainerName))
	if err != nil {
		s.T().Logf("Failed to run snmpwalk: %v", err)
	}
	s.T().Logf("SNMP result:\n%s", snmpResult)

	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		output := agent.Client.Status(agentclient.WithArgs([]string{"snmp"}))
		s.T().Logf("Agent status: %s", output.Content)

		assert.Contains(t, output.Content, "Subnet 192.168.100.0/30 scanned.")
		assert.Contains(t, output.Content, "- 192.168.100.2")
		assert.Contains(t, output.Content, "Subnet 192.168.101.0/30 scanned.")
		assert.NotContains(t, output.Content, "- 192.168.101.2")

		deviceMetrics, err := fakeIntake.FilterMetrics("snmp.devices_monitored")
		assert.NoError(t, err)
		assert.NotEmpty(t, deviceMetrics, "No SNMP devices_monitored metric found")
		//s.T().Logf("deviceMetrics: %v", deviceMetrics)
		if len(deviceMetrics) > 0 && len(deviceMetrics[0].Points) > 0 {
			devicesMonitored := deviceMetrics[0].Points[0].Value
			assert.Equal(t, float64(1), devicesMonitored,
				"Expected exactly 1 monitored device, got %.0f (deduplication not working)",
				devicesMonitored)
		}
	}, 1*time.Minute, 20*time.Second)

}
