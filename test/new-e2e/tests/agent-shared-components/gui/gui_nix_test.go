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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"
)

type guiLinuxSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestGUILinuxSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &guiLinuxSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake()))
}

func (v *guiLinuxSuite) TestGUI() {
	authTokenFilePath := "/etc/datadog-agent/auth_token"

	config := fmt.Sprintf(`auth_token_file_path: %v
cmd_port: %d
GUI_port: %d`, authTokenFilePath, agentAPIPort, guiPort)
	// start the agent with that configuration
	v.UpdateEnv(awshost.Provisioner(
		awshost.WithAgentOptions(
			agentparams.WithAgentConfig(config),
		),
		awshost.WithAgentClientOptions(
			agentclientparams.WithAuthTokenPath(authTokenFilePath),
		),
	))

	// get auth token
	v.T().Log("Getting the authentication token")
	authtokenContent := v.Env().RemoteHost.MustExecute("sudo cat " + authTokenFilePath)
	authtoken := strings.TrimSpace(authtokenContent)

	v.T().Log("Testing GUI authentication flow")

	var guiClient *http.Client
	// and check that the agents are using the new key
	require.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		guiClient = getGUIClient(t, v.Env().RemoteHost, authtoken)
	}, 30*time.Second, 5*time.Second)

	v.T().Log("Testing GUI static file server")
	checkStaticFiles(v.T(), guiClient, v.Env().RemoteHost, "/opt/datadog-agent")

	v.T().Log("Testing GUI ping endpoint")
	checkPingEndpoint(v.T(), guiClient)
}
