// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package upgrade

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	filemanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/file-manager"
	helpers "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/helper"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install/installparams"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/platforms"
	e2eOs "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/stretchr/testify/require"
	"os"
	"strings"
	"testing"
)

var osVersion = flag.String("osversion", "", "os version to test")
var platform = flag.String("platform", "", "platform to test")
var architecture = flag.String("arch", "", "architecture to test (x86_64, arm64))")
var flavorName = flag.String("flavor", "datadog-agent", "package flavor to install")
var srcAgentVersion = flag.String("src-agent-version", "5", "start agent version")
var destAgentVersion = flag.String("dest-agent-version", "7", "destination agent version to upgrade to")

type upgradeSuite struct {
	e2e.Suite[e2e.VMEnv]
	srcVersion  string
	destVersion string
}

func TestUpgradeScript(t *testing.T) {
	osMapping := map[string]ec2os.Type{
		"debian":      ec2os.DebianOS,
		"ubuntu":      ec2os.UbuntuOS,
		"centos":      ec2os.CentOS,
		"amazonlinux": ec2os.AmazonLinuxOS,
		"redhat":      ec2os.RedHatOS,
		"rhel":        ec2os.RedHatOS,
		"sles":        ec2os.SuseOS,
		"windows":     ec2os.WindowsOS,
		"fedora":      ec2os.FedoraOS,
		"suse":        ec2os.SuseOS,
		"rocky":       ec2os.RockyLinux,
	}

	archMapping := map[string]e2eOs.Architecture{
		"x86_64": e2eOs.AMD64Arch,
		"arm64":  e2eOs.ARM64Arch,
	}

	platformJSON := map[string]map[string]map[string]string{}

	err := json.Unmarshal(platforms.Content, &platformJSON)
	require.NoErrorf(t, err, "failed to umarshall platform file: %v", err)

	osVersions := strings.Split(*osVersion, ",")
	fmt.Println("Parsed platform json file: ", platformJSON)
	for _, osVers := range osVersions {
		vmOpts := []ec2params.Option{}
		osVers := osVers

		var testOsType ec2os.Type
		for osName, osType := range osMapping {
			if strings.Contains(osVers, osName) {
				testOsType = osType
			}
		}

		t.Run(fmt.Sprintf("test upgrade on %s %s", osVers, *architecture), func(tt *testing.T) {
			tt.Parallel()
			fmt.Printf("Testing %s", osVers)

			vmOpts = append(vmOpts, ec2params.WithImageName(platformJSON[*platform][*architecture][osVers], archMapping[*architecture], testOsType))
			if instanceType, ok := os.LookupEnv("E2E_OVERRIDE_INSTANCE_TYPE"); ok {
				vmOpts = append(vmOpts, ec2params.WithInstanceType(instanceType))
			}

			e2e.Run(tt, &upgradeSuite{srcVersion: *srcAgentVersion, destVersion: *destAgentVersion}, e2e.EC2VMStackDef(vmOpts...), params.WithStackName(fmt.Sprintf("upgrade-from-%s-to-%s-test-%s-%v-%v-%s", *srcAgentVersion, *destAgentVersion, *flavorName, os.Getenv("CI_PIPELINE_ID"), osVers, *architecture)))
		})
	}
}

func (is *upgradeSuite) TestUpgrade() {
	fileManager := filemanager.NewUnixFileManager(is.Env().VM)

	vm := is.Env().VM.(*client.PulumiStackVM)
	agentClient, err := client.NewAgentClient(is.T(), vm, vm.GetOS(), false)
	require.NoError(is.T(), err)

	unixHelper := helpers.NewUnixHelper()
	VMclient := common.NewTestClient(is.Env().VM, agentClient, fileManager, unixHelper)
	is.SetupAgentStartVersion(VMclient)
	is.UpgradeAgentVersion(VMclient)
	is.CheckUpgradeAgentInstallation(VMclient)

}

func (is *upgradeSuite) SetupAgentStartVersion(VMclient *common.TestClient) {
	install.Unix(is.T(), VMclient, installparams.WithArch(*architecture), installparams.WithFlavor(*flavorName), installparams.WithMajorVersion(is.srcVersion), installparams.WithAPIKey(os.Getenv("DATADOG_AGENT_API_KEY")), installparams.WithPipelineID(""))
	var err error
	if is.srcVersion == "5" {
		_, err = VMclient.VMClient.ExecuteWithError("sudo /etc/init.d/datadog-agent stop")
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
