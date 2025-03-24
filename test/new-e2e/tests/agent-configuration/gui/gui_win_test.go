// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package gui

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	msiparams "github.com/DataDog/test-infra-definitions/components/datadog/agentparams/msi"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"
)

const authTokenFilePath = `C:\ProgramData\Datadog\auth_token`
const installPath = `c:\Program Files\CustomPath\Datadog Agent`

var config = fmt.Sprintf(`auth_token_file_path: %v
cmd_port: %d
GUI_port: %d`, authTokenFilePath, agentAPIPort, guiPort)

type guiWindowsSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestGUIWindowsSuite(t *testing.T) {
	t.Parallel()

	e2e.Run(t, &guiWindowsSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(
		awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
		awshost.WithAgentOptions(
			agentparams.WithAgentConfig(config),
			agentparams.WithAdditionalInstallParameters(
				msiparams.NewInstallParams(
					msiparams.WithCustomInstallPath(fmt.Sprintf(`"%s"`, installPath)),
				),
			),
		),
		awshost.WithAgentClientOptions(
			agentclientparams.WithAuthTokenPath(authTokenFilePath),
		),
	)))
}

func (v *guiWindowsSuite) TestGUI() {
	// get auth token
	v.T().Log("Getting the authentication token")
	authtokenContent, err := v.Env().RemoteHost.ReadFile(authTokenFilePath)
	require.NoError(v.T(), err)

	authtoken := strings.TrimSpace(string(authtokenContent))

	v.T().Log("Trying to connect to GUI server")

	var guiClient *http.Client
	// and check that the agents are using the new key
	require.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		guiClient = getGUIClient(t, v.Env().RemoteHost, authtoken)
	}, 1*time.Minute, 10*time.Second)

	v.T().Log("Testing GUI static file server")
	checkStaticFiles(v.T(), guiClient, v.Env().RemoteHost, installPath)

	v.T().Log("Testing GUI ping endpoint")
	checkPingEndpoint(v.T(), guiClient)
}
