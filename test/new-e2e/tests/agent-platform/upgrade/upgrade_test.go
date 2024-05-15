// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package upgrade

import (
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

var (
	osVersion        = flag.String("osversion", "", "os version to test")
	platform         = flag.String("platform", "", "platform to test")
	architecture     = flag.String("arch", "", "architecture to test (x86_64, arm64))")
	flavorName       = flag.String("flavor", "datadog-agent", "package flavor to install")
	srcAgentVersion  = flag.String("src-agent-version", "5", "start agent version")
	destAgentVersion = flag.String("dest-agent-version", "7", "destination agent version to upgrade to")
)

type upgradeSuite struct {
	e2e.BaseSuite[environments.Host]
	srcVersion  string
	destVersion string
}

func TestUpgradeScript(t *testing.T) {
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

		t.Run(fmt.Sprintf("test upgrade on %s %s", osVers, *architecture), func(tt *testing.T) {
			flake.Mark(tt)
			tt.Parallel()
			tt.Logf("Testing %s", osVers)

			osDesc := platforms.BuildOSDescriptor(*platform, *architecture, osVers)
			vmOpts = append(vmOpts, ec2.WithAMI(platformJSON[*platform][*architecture][osVers], osDesc, osDesc.Architecture))

			e2e.Run(tt,
				&upgradeSuite{srcVersion: *srcAgentVersion, destVersion: *destAgentVersion},
				e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake(
					awshost.WithEC2InstanceOptions(vmOpts...),
				)),
				e2e.WithStackName(fmt.Sprintf("upgrade-from-%s-to-%s-test-%s-%v-%v-%s", *srcAgentVersion, *destAgentVersion, *flavorName, os.Getenv("CI_PIPELINE_ID"), osVers, *architecture)),
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
	is.CheckUpgradeAgentInstallation(VMclient)
}

func (is *upgradeSuite) SetupAgentStartVersion(VMclient *common.TestClient) {
	install.Unix(is.T(), VMclient, installparams.WithArch(*architecture), installparams.WithFlavor(*flavorName), installparams.WithMajorVersion(is.srcVersion), installparams.WithAPIKey(os.Getenv("DATADOG_AGENT_API_KEY")), installparams.WithPipelineID(""))
	var err error
	if is.srcVersion == "5" {
		_, err = VMclient.Host.Execute("sudo /etc/init.d/datadog-agent stop")
	} else {
		_, err = VMclient.SvcManager.Stop("datadog-agent")
	}
	require.NoError(is.T(), err)
}

func (is *upgradeSuite) UpgradeAgentVersion(VMclient *common.TestClient) {
	install.Unix(is.T(), VMclient, installparams.WithArch(*architecture), installparams.WithFlavor(*flavorName), installparams.WithMajorVersion(is.destVersion), installparams.WithUpgrade(true))
	_, err := VMclient.SvcManager.Restart("datadog-agent")
	require.NoError(is.T(), err)
}

func (is *upgradeSuite) CheckUpgradeAgentInstallation(VMclient *common.TestClient) {
	common.CheckInstallation(is.T(), VMclient)
	common.CheckInstallationMajorAgentVersion(is.T(), VMclient, is.destVersion)
}
