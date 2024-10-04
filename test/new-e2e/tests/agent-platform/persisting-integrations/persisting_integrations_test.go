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

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	filemanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/file-manager"
	helpers "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/helper"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install/installparams"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/platforms"

	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/stretchr/testify/require"
)

// TODO: This is a temporary workaround to test the upgrade of the persisting integrations
// Since we need the previous agent version to have produced these files
// We will mock these files for now since this is the first release that will generate them
// Once we have a release that generates these files we can remove this workaround and stop mocking these files

//go:embed fixtures/diff.txt
var diffPythonInstalledPackages string

//go:embed fixtures/postinst.txt
var postinstPythonInstalledPackages string

//go:embed fixtures/requirements.txt
var requirementsAgentRelease string

var (
	osVersion       = flag.String("osversion", "", "os version to test")
	platform        = flag.String("platform", "", "platform to test")
	architecture    = flag.String("arch", "", "architecture to test (x86_64, arm64))")
	flavorName      = flag.String("flavor", "datadog-agent", "package flavor to install")
	srcAgentVersion = flag.String("src-agent-version", "7", "start agent version")
)

var filesToMock = map[string]string{
	"/opt/datadog-agent/.diff_python_installed_packages.txt":     diffPythonInstalledPackages,
	"/opt/datadog-agent/.postinst_python_installed_packages.txt": postinstPythonInstalledPackages,
	"/opt/datadog-agent/requirements-agent-release.txt":          requirementsAgentRelease,
}

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
			flake.Mark(tt)
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

func (is *persistingIntegrationsSuite) TestIntegrationPersistsWithFileFlag() {
	VMclient := is.SetupTestClient()

	startAgentVersion := is.SetupAgentStartVersion(VMclient)
	is.InstallNVMLIntegration(VMclient)
	is.PrepareMockedFiles(VMclient)

	// set the flag to install third party deps
	is.EnableInstallThirdPartyDepsFlag(VMclient)

	upgradedAgentVersion := is.UpgradeAgentVersion(VMclient)
	is.Require().NotEqual(startAgentVersion, upgradedAgentVersion)
	is.CheckIntegrationInstalled(VMclient)
}

func (is *persistingIntegrationsSuite) TestIntegrationDoesNotPersistWithoutFileFlag() {
	VMclient := is.SetupTestClient()

	startAgentVersion := is.SetupAgentStartVersion(VMclient)
	is.InstallNVMLIntegration(VMclient)
	is.PrepareMockedFiles(VMclient)

	// unset the flag to install third party deps if it was set
	VMclient.Host.Execute("sudo rm -f /opt/datadog-agent/.install_python_third_party_deps")

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

func (is *persistingIntegrationsSuite) PrepareMockedFiles(VMclient *common.TestClient) {
	for file, content := range filesToMock {
		VMclient.Host.MustExecute(fmt.Sprintf("sudo bash -c \"echo '%s' > %s\"", content, file))
	}
}

func (is *persistingIntegrationsSuite) InstallNVMLIntegration(VMclient *common.TestClient) {
	// Make sure that the integration is not installed
	stdout := VMclient.Host.MustExecute("sudo runuser -u dd-agent -- datadog-agent integration freeze")
	is.Assert().NotContains(stdout, "datadog-nvml")

	// Install the integration and its dependencies
	VMclient.Host.MustExecute("sudo runuser -u dd-agent -- datadog-agent integration install -t datadog-nvml==1.0.0")
	VMclient.Host.MustExecute("sudo runuser -u dd-agent -- /opt/datadog-agent/embedded/bin/pip3 install grpcio pynvml")

	// Check that the integration is installed successfully
	stdout = VMclient.Host.MustExecute("sudo runuser -u dd-agent -- datadog-agent integration freeze")
	is.Require().Contains(stdout, "datadog-nvml==1.0.0")
}

func (is *persistingIntegrationsSuite) GetAgentVersion(VMclient *common.TestClient) string {
	return VMclient.Host.MustExecute("sudo runuser -u dd-agent -- datadog-agent version")
}

func (is *persistingIntegrationsSuite) EnableInstallThirdPartyDepsFlag(VMclient *common.TestClient) string {
	return VMclient.Host.MustExecute("sudo touch /opt/datadog-agent/.install_python_third_party_deps")
}

func (is *persistingIntegrationsSuite) SetupAgentStartVersion(VMclient *common.TestClient) string {
	// By default, pipelineID is set to E2E_PIPELINE_ID, we need to unset it to avoid installing the agent from the pipeline
	install.Unix(is.T(), VMclient, installparams.WithArch(*architecture), installparams.WithFlavor(*flavorName), installparams.WithMajorVersion(is.srcVersion), installparams.WithAPIKey(os.Getenv("DATADOG_AGENT_API_KEY")), installparams.WithPipelineID(""))
	common.CheckInstallation(is.T(), VMclient)
	return is.GetAgentVersion(VMclient)
}

func (is *persistingIntegrationsSuite) UpgradeAgentVersion(VMclient *common.TestClient) string {
	// Unset/Reset sticky bit on /tmp to allow the agent to write the error log
	defer VMclient.Host.MustExecute("sudo chmod +t /tmp")
	VMclient.Host.MustExecute("sudo chmod -t /tmp")

	install.Unix(is.T(), VMclient, installparams.WithArch(*architecture), installparams.WithFlavor(*flavorName), installparams.WithUpgrade(true))

	common.CheckInstallation(is.T(), VMclient)

	return is.GetAgentVersion(VMclient)
}

func (is *persistingIntegrationsSuite) CheckIntegrationInstalled(VMclient *common.TestClient) {
	stdout := VMclient.Host.MustExecute("sudo runuser -u dd-agent -- datadog-agent integration freeze")
	is.Assert().Contains(stdout, "datadog-nvml==1.0.0")
}

func (is *persistingIntegrationsSuite) CheckIntegrationNotInstalled(VMclient *common.TestClient) {
	stdout := VMclient.Host.MustExecute("sudo runuser -u dd-agent -- datadog-agent integration freeze")
	is.Assert().NotContains(stdout, "datadog-nvml")
}
