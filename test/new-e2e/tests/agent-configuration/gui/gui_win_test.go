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

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	msiparams "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams/msi"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclientparams"
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
		awshost.WithRunOptions(
			ec2.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault)),
			ec2.WithAgentOptions(
				agentparams.WithAgentConfig(config),
				agentparams.WithAdditionalInstallParameters(
					msiparams.NewInstallParams(
						msiparams.WithCustomInstallPath(fmt.Sprintf(`"%s"`, installPath)),
					),
				),
			),
			ec2.WithAgentClientOptions(
				agentclientparams.WithAuthTokenPath(authTokenFilePath),
			),
		))))
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
		guiClient = getGUIClient(t, v.Env().RemoteHost, authtoken, ipcCertPathWin)
	}, 1*time.Minute, 10*time.Second)

	v.T().Log("Testing GUI static file server")
	checkStaticFiles(v.T(), guiClient, v.Env().RemoteHost, installPath)

	v.T().Log("Testing GUI ping endpoint")
	checkPingEndpoint(v.T(), guiClient)
}
