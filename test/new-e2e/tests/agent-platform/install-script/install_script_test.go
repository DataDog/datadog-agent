// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package installscript

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	filemanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/file-manager"
	helpers "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/helper"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/platforms"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install"
	e2eOs "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"

	"testing"
)

var osVersion = flag.String("osversion", "", "os version to test")
var platform = flag.String("platform", "", "platform to test")
var cwsSupportedOsVersion = flag.String("cws-supported-osversion", "", "list of os where CWS is supported")

type installScriptSuite struct {
	e2e.Suite[e2e.VMEnv]
	cwsSupported bool
}

func TestInstallScript(t *testing.T) {
	osMapping := map[string]ec2os.Type{
		"debian":      ec2os.DebianOS,
		"ubuntu":      ec2os.UbuntuOS,
		"centos":      ec2os.CentOS,
		"amazonlinux": ec2os.AmazonLinuxOS,
		"redhat":      ec2os.RedHatOS,
		"windows":     ec2os.WindowsOS,
		"fedora":      ec2os.FedoraOS,
		"suse":        ec2os.SuseOS,
	}
	platformJSON := map[string]map[string]string{}

	err := json.Unmarshal(platforms.Content, &platformJSON)
	require.NoErrorf(t, err, "failed to umarshall platform file: %v", err)

	osVersions := strings.Split(*osVersion, ",")
	cwsSupportedOsVersionList := strings.Split(*cwsSupportedOsVersion, ",")

	fmt.Println("Parsed platform json file: ", platformJSON)
	for _, osVers := range osVersions {
		osVers := osVers
		cwsSupported := false
		for _, cwsSupportedOs := range cwsSupportedOsVersionList {
			if cwsSupportedOs == osVers {
				cwsSupported = true
			}
		}

		t.Run(fmt.Sprintf("test install script on %s", osVers), func(tt *testing.T) {
			tt.Parallel()
			fmt.Printf("Testing %s", osVers)
			e2e.Run(tt, &installScriptSuite{cwsSupported: cwsSupported}, e2e.EC2VMStackDef(ec2params.WithImageName(platformJSON[*platform][osVers], e2eOs.AMD64Arch, osMapping[*platform])), params.WithStackName(fmt.Sprintf("install-script-test-%v-%v", os.Getenv("CI_PIPELINE_ID"), osVers)))
		})
	}
}

func (is *installScriptSuite) TestInstallAgent() {
	fileManager := filemanager.NewUnixFileManager(is.Env().VM)
	unixHelper := helpers.NewUnixHelper()
	vm := is.Env().VM.(*client.PulumiStackVM)
	agentClient, err := client.NewAgentClient(is.T(), vm, vm.GetOS(), false)
	require.NoError(is.T(), err)
	client := common.NewTestClient(is.Env().VM, agentClient, fileManager, unixHelper)

	install.Unix(is.T(), client)

	common.CheckInstallation(is.T(), client)
	common.CheckAgentBehaviour(is.T(), client)
	common.CheckAgentStops(is.T(), client)
	common.CheckAgentRestarts(is.T(), client)
	common.CheckIntegrationInstall(is.T(), client)
	common.CheckAgentPython(is.T(), client, "3")
	common.CheckApmEnabled(is.T(), client)
	common.CheckApmDisabled(is.T(), client)
	if is.cwsSupported {
		common.CheckCWSBehaviour(is.T(), client)
	}
	common.CheckInstallationInstallScript(is.T(), client)
	common.CheckUninstallation(is.T(), client)

}
