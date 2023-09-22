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

	"github.com/DataDog/datadog-agent/test/new-e2e/agent-platform/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	e2eOs "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"

	"testing"
)

var osVersion = flag.String("osversion", "", "os version to test")
var platform = flag.String("platform", "", "platform to test")
var cwsSupportedOsVersion = flag.String("cws-supported-osversion", "", "list of os where CWS is supported")

type installScriptSuite struct {
	e2e.Suite[e2e.AgentEnv]
	cwsSupported bool
}

func TestInstallScript(t *testing.T) {
	platformJSON := map[string]map[string]string{}
	platformFileContent, err := os.ReadFile("../platforms.json")
	if err != nil {
		panic(fmt.Sprintf("failed to read platform file: %v", err))
	}
	err = json.Unmarshal(platformFileContent, &platformJSON)
	if err != nil {
		panic(fmt.Sprintf("failed to umarshall platform file: %v", err))
	}
	osVersions := strings.Split(*osVersion, ",")
	cwsSupportedOsVersionList := strings.Split(*cwsSupportedOsVersion, ",")

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
			e2e.Run(tt, &installScriptSuite{cwsSupported: cwsSupported}, e2e.AgentStackDef(e2e.WithVMParams(ec2params.WithImageName(platformJSON[*platform][osVers], e2eOs.AMD64Arch, ec2os.DebianOS)), e2e.WithAgentParams(agentparams.WithAgentConfig("site: datadoghq.eu"))), params.WithStackName(fmt.Sprintf("install-script-test-%v-%v", os.Getenv("CI_PIPELINE_ID"), osVers)))
		})
	}
}

func (is *installScriptSuite) TestInstallAgent() {

	client := common.NewClientFromEnv(is.Env())

	common.CheckInstallation(is.T(), client)
	common.CheckAgentBehaviour(is.T(), client)
	common.CheckIntegrationInstall(is.T(), client)
	common.CheckAgentStops(is.T(), client)
	common.CheckAgentRestarts(is.T(), client)
	common.CheckAgentPython(is.T(), client, "3")
	common.CheckApmEnabled(is.T(), client)
	common.CheckApmDisabled(is.T(), client)
	if is.cwsSupported {
		common.CheckCWSBehaviour(is.T(), client)
	}
	common.CheckInstallationInstallScript(is.T(), client)
	common.CheckUninstallation(is.T(), client)

}
