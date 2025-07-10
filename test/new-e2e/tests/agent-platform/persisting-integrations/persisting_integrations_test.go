// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package persistingintegrations

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	filemanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/file-manager"
	helpers "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/helper"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install/installparams"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/platforms"

	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/stretchr/testify/require"
)

var (
	osVersion       = flag.String("osversion", "", "os version to test")
	platform        = flag.String("platform", "", "platform to test")
	architecture    = flag.String("arch", "", "architecture to test (x86_64, arm64))")
	flavorName      = flag.String("flavor", "datadog-agent", "package flavor to install")
	srcAgentVersion = flag.String("src-agent-version", "7", "start agent version")
)

type persistingIntegrationsSuite struct {
	e2e.BaseSuite[environments.Host]
	srcVersion string
	platform   string
}

func (is *persistingIntegrationsSuite) AfterTest(suiteName, testName string) {
	is.BaseSuite.AfterTest(suiteName, testName)
	platform := strings.ToLower(is.platform)

	if platform == "ubuntu" || platform == "debian" {
		is.Env().RemoteHost.Execute("sudo apt-get remove datadog-agent -y")
		is.Env().RemoteHost.MustExecute("sudo apt-get remove --purge datadog-agent -y")
	} else if platform == "redhat" || platform == "amazonlinux" || platform == "centos" {
		is.Env().RemoteHost.MustExecute("sudo yum remove datadog-agent -y")
		is.Env().RemoteHost.Execute("sudo userdel dd-agent")
		is.Env().RemoteHost.Execute("sudo rm -rf /opt/datadog-agent/")
		is.Env().RemoteHost.Execute("sudo rm -rf /etc/datadog-agent/")
		is.Env().RemoteHost.Execute("sudo rm -rf /var/log/datadog/")
	} else if platform == "suse" {
		is.Env().RemoteHost.MustExecute("sudo zypper remove datadog-agent")
		is.Env().RemoteHost.Execute("sudo userdel dd-agent")
		is.Env().RemoteHost.Execute("sudo rm -rf /opt/datadog-agent/")
		is.Env().RemoteHost.Execute("sudo rm -rf /etc/datadog-agent/")
		is.Env().RemoteHost.Execute("sudo rm -rf /var/log/datadog/")
	} else {
		is.T().Fatal("Unsupported platform for cleanup")
	}
}

func TestPersistingIntegrations(t *testing.T) {
	platformJSON := map[string]map[string]map[string]string{}

	err := json.Unmarshal(platforms.Content, &platformJSON)
	require.NoErrorf(t, err, "failed to umarshall platform file: %v", err)

	osVersions := strings.Split(*osVersion, ",")
	t.Log("Parsed platform json file: ", platformJSON)

	vmOpts := []ec2.VMOption{}
	if instanceType, ok := os.LookupEnv("E2E_OVERRIDE_INSTANCE_TYPE"); ok {
		vmOpts = append(vmOpts, ec2.WithInstanceType(instanceType))
	}

	for _, osVers := range osVersions {
		osVers := osVers
		if platformJSON[*platform][*architecture][osVers] == "" {
			// Fail if the image is not defined instead of silently running with default Ubuntu AMI
			t.Fatalf("No image found for %s %s %s", *platform, *architecture, osVers)
		}

		t.Run(fmt.Sprintf("test upgrade persisting integrations on %s %s", osVers, *architecture), func(tt *testing.T) {
			tt.Parallel()
			tt.Logf("Testing %s", osVers)

			osDesc := platforms.BuildOSDescriptor(*platform, *architecture, osVers)
			vmOpts = append(vmOpts, ec2.WithAMI(platformJSON[*platform][*architecture][osVers], osDesc, osDesc.Architecture))

			e2e.Run(tt,
				&persistingIntegrationsSuite{srcVersion: *srcAgentVersion, platform: *platform},
				e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake(
					awshost.WithEC2InstanceOptions(vmOpts...),
				)),
				e2e.WithStackName(fmt.Sprintf("upgrade-persisting-integrations-%s-test-%s-%v-%s", *srcAgentVersion, *flavorName, osVers, *architecture)),
			)
		})
	}
}

func (is *persistingIntegrationsSuite) TestIntegrationPersistsByDefault() {
	VMclient := is.SetupTestClient()

	startAgentVersion := is.SetupAgentStartVersion(VMclient)
	is.InstallNVMLIntegration(VMclient)

	// remove the flag to skip installing third party deps if it exists
	is.DisableSkipInstallThirdPartyDepsFlag(VMclient)

	upgradedAgentVersion := is.UpgradeAgentVersion(VMclient)
	is.Require().NotEqual(startAgentVersion, upgradedAgentVersion)
	is.CheckIntegrationInstalled(VMclient)
}

func (is *persistingIntegrationsSuite) TestIntegrationDoesNotPersistWithSkipFileFlag() {
	VMclient := is.SetupTestClient()

	startAgentVersion := is.SetupAgentStartVersion(VMclient)
	is.InstallNVMLIntegration(VMclient)

	// set the flag to skip installing third party deps
	is.EnableSkipInstallThirdPartyDepsFlag(VMclient)

	upgradedAgentVersion := is.UpgradeAgentVersion(VMclient)
	is.Require().NotEqual(startAgentVersion, upgradedAgentVersion)
	is.CheckIntegrationNotInstalled(VMclient)
}

func (is *persistingIntegrationsSuite) SetupTestClient() *common.TestClient {
	fileManager := filemanager.NewUnix(is.Env().RemoteHost)
	agentClient, err := client.NewHostAgentClient(is, is.Env().RemoteHost.HostOutput, false)
	require.NoError(is.T(), err)
	unixHelper := helpers.NewUnix()
	VMclient := common.NewTestClient(is.Env().RemoteHost, agentClient, fileManager, unixHelper)
	return VMclient
}

func (is *persistingIntegrationsSuite) InstallNVMLIntegration(VMclient *common.TestClient) {
	// Make sure that the integration is not installed
	freezeRequirement := VMclient.AgentClient.Integration(agentclient.WithArgs([]string{"freeze"}))
	is.Assert().NotContains(freezeRequirement, "datadog-nvml")

	// Install the integration and its dependencies
	VMclient.Host.MustExecute("sudo -u dd-agent datadog-agent integration install -t datadog-nvml==1.0.0")
	VMclient.Host.MustExecute("sudo -u dd-agent /opt/datadog-agent/embedded/bin/pip3 install grpcio pynvml")

	// Check that the integration is installed successfully
	freezeRequirement = VMclient.AgentClient.Integration(agentclient.WithArgs([]string{"freeze"}))
	is.Require().Contains(freezeRequirement, "datadog-nvml==1.0.0")
}

func (is *persistingIntegrationsSuite) EnableSkipInstallThirdPartyDepsFlag(VMclient *common.TestClient) string {
	return VMclient.Host.MustExecute("sudo touch /etc/datadog-agent/.skip_install_python_third_party_deps")
}

func (is *persistingIntegrationsSuite) DisableSkipInstallThirdPartyDepsFlag(VMclient *common.TestClient) (string, error) {
	return VMclient.Host.Execute("sudo rm -f /etc/datadog-agent/.skip_install_python_third_party_deps")
}

func (is *persistingIntegrationsSuite) SetupAgentStartVersion(VMclient *common.TestClient) string {
	// By default, pipelineID is set to E2E_PIPELINE_ID, we need to unset it to avoid installing the agent from the pipeline
	install.Unix(is.T(), VMclient, installparams.WithArch(*architecture), installparams.WithFlavor(*flavorName), installparams.WithMajorVersion(is.srcVersion), installparams.WithAPIKey(os.Getenv("DATADOG_AGENT_API_KEY")), installparams.WithPipelineID(""))
	common.CheckInstallation(is.T(), VMclient)
	return VMclient.AgentClient.Version()
}

func (is *persistingIntegrationsSuite) UpgradeAgentVersion(VMclient *common.TestClient) string {
	// Unset/Reset sticky bit on /tmp to allow the agent to write the error log
	defer VMclient.Host.MustExecute("sudo chmod +t /tmp")
	VMclient.Host.MustExecute("sudo chmod -t /tmp")

	install.Unix(is.T(), VMclient, installparams.WithArch(*architecture), installparams.WithFlavor(*flavorName), installparams.WithUpgrade(true))

	common.CheckInstallation(is.T(), VMclient)

	return VMclient.AgentClient.Version()
}

func (is *persistingIntegrationsSuite) CheckIntegrationInstalled(VMclient *common.TestClient) {
	freezeRequirement := VMclient.AgentClient.Integration(agentclient.WithArgs([]string{"freeze"}))
	is.Assert().Contains(freezeRequirement, "datadog-nvml==1.0.0")
}

func (is *persistingIntegrationsSuite) CheckIntegrationNotInstalled(VMclient *common.TestClient) {
	freezeRequirement := VMclient.AgentClient.Integration(agentclient.WithArgs([]string{"freeze"}))
	is.Assert().NotContains(freezeRequirement, "datadog-nvml")
}
