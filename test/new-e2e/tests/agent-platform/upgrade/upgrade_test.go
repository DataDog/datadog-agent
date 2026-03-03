// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package upgrade

import (
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	filemanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/file-manager"
	helpers "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/helper"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install/installparams"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/platforms"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/stretchr/testify/require"
)

var (
	osDescriptors    = flag.String("osdescriptors", "", "platform/arch/os version (debian/x86_64/11)")
	flavorName       = flag.String("flavor", "datadog-agent", "package flavor to install")
	srcAgentVersion  = flag.String("src-agent-version", "5", "start agent version")
	destAgentVersion = flag.String("dest-agent-version", "7", "destination agent version to upgrade to")
)

type upgradeSuite struct {
	e2e.BaseSuite[environments.Host]
	osDesc         e2eos.Descriptor
	srcVersion     string
	destVersion    string
	testingKeysURL string
}

func TestUpgradeScript(t *testing.T) {
	osDescriptors, err := platforms.ParseOSDescriptors(*osDescriptors)
	if err != nil {
		t.Fatalf("failed to parse os descriptors: %v", err)
	}
	if len(osDescriptors) == 0 {
		t.Fatal("expecting some value to be passed for --osdescriptors on test invocation, got none")
	}

	vmOpts := []ec2.VMOption{}
	if instanceType, ok := os.LookupEnv("E2E_OVERRIDE_INSTANCE_TYPE"); ok {
		vmOpts = append(vmOpts, ec2.WithInstanceType(instanceType))
	}

	for _, osDesc := range osDescriptors {
		osDesc := osDesc

		t.Run("test upgrade on "+platforms.PrettifyOsDescriptor(osDesc), func(tt *testing.T) {
			tt.Parallel()
			tt.Logf("Testing %s", platforms.PrettifyOsDescriptor(osDesc))

			vmOpts = append(vmOpts, ec2.WithOS(osDesc))

			suite := &upgradeSuite{srcVersion: *srcAgentVersion, destVersion: *destAgentVersion, osDesc: osDesc}
			suite.testingKeysURL = "apttesting.datad0g.com/test-keys"

			e2e.Run(tt,
				suite,
				e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake(
					awshost.WithRunOptions(ec2.WithEC2InstanceOptions(vmOpts...)),
				)),
				e2e.WithStackName(fmt.Sprintf("upgrade-from-%s-to-%s-test-%s-%s", *srcAgentVersion, *destAgentVersion, *flavorName, platforms.PrettifyOsDescriptor(osDesc))),
			)
		})
	}
}

func (is *upgradeSuite) TestUpgrade() {
	fileManager := filemanager.NewUnix(is.Env().RemoteHost)

	agentClient, err := client.NewHostAgentClient(is, is.Env().RemoteHost.HostOutput, false)
	require.NoError(is.T(), err)

	unixHelper := helpers.NewUnix()
	VMclient := common.NewTestClient(is.Env().RemoteHost, agentClient, fileManager, unixHelper)
	is.SetupAgentStartVersion(VMclient)
	is.UpgradeAgentVersion(VMclient)
	time.Sleep(5 * time.Second) // Restarting the agent too fast will cause systemctl to fail
	is.CheckUpgradeAgentInstallation(VMclient)
}

func (is *upgradeSuite) SetupAgentStartVersion(VMclient *common.TestClient) {
	installOptions := []installparams.Option{
		installparams.WithArch(string(is.osDesc.Architecture)),
		installparams.WithFlavor(*flavorName),
		installparams.WithMajorVersion(is.srcVersion),
		installparams.WithAPIKey(os.Getenv("DATADOG_AGENT_API_KEY")),
		installparams.WithPipelineID(""),
	}
	if is.testingKeysURL != "" {
		installOptions = append(installOptions, installparams.WithTestingKeysURL(is.testingKeysURL))
	}
	install.Unix(is.T(), VMclient, installOptions...)
	var err error
	if is.srcVersion == "5" {
		_, err = VMclient.Host.Execute("sudo /etc/init.d/datadog-agent stop")
	} else {
		_, err = VMclient.SvcManager.Stop("datadog-agent")
	}
	require.NoError(is.T(), err)
}

func (is *upgradeSuite) UpgradeAgentVersion(VMclient *common.TestClient) {
	installOptions := []installparams.Option{
		installparams.WithArch(string(is.osDesc.Architecture)),
		installparams.WithFlavor(*flavorName),
		installparams.WithMajorVersion(is.destVersion),
		installparams.WithUpgrade(true),
	}

	if is.testingKeysURL != "" {
		installOptions = append(installOptions, installparams.WithTestingKeysURL(is.testingKeysURL))
	}
	install.Unix(is.T(), VMclient, installOptions...)
	_, err := VMclient.SvcManager.Restart("datadog-agent")
	require.NoError(is.T(), err)
}

func (is *upgradeSuite) CheckUpgradeAgentInstallation(VMclient *common.TestClient) {
	common.CheckInstallation(is.T(), VMclient)
	common.CheckInstallationMajorAgentVersion(is.T(), VMclient, is.destVersion)
}
