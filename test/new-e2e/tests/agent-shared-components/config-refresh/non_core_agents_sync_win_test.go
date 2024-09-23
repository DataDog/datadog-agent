// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package configrefresh

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"
	secrets "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-shared-components/secretsutils"
)

type configRefreshWindowsSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestConfigRefreshWindowsSuite(t *testing.T) {
	// WINA-1014
	flake.Mark(t)

	t.Parallel()
	e2e.Run(t, &configRefreshWindowsSuite{}, e2e.WithProvisioner(awshost.Provisioner(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)))))
}

func (v *configRefreshWindowsSuite) TestConfigRefresh() {
	rootDir := "C:/tmp/" + v.T().Name()
	v.Env().RemoteHost.MkdirAll(rootDir)

	authTokenFilePath := `C:\ProgramData\Datadog\auth_token`
	secretResolverPath := filepath.Join(rootDir, "wrapper.bat")

	v.T().Log("Setting up the secret resolver and the initial api key file")

	secretClient := secrets.NewSecretClient(v.T(), v.Env().RemoteHost, rootDir)
	secretClient.SetSecret("api_key", apiKey1)

	templateVars := map[string]interface{}{
		"AuthTokenFilePath":        authTokenFilePath,
		"SecretDirectory":          rootDir,
		"SecretResolver":           secretResolverPath,
		"ConfigRefreshIntervalSec": configRefreshIntervalSec,
		"ApmCmdPort":               apmCmdPort,
		"ProcessCmdPort":           processCmdPort,
		"SecurityCmdPort":          securityCmdPort,
		"AgentIpcPort":             agentIpcPort,
		"SecretBackendCommandAllowGroupExecPermOption": "false", // this is not supported on Windows
	}
	coreconfig := fillTmplConfig(v.T(), coreConfigTmpl, templateVars)

	agentOptions := []func(*agentparams.Params) error{
		agentparams.WithAgentConfig(coreconfig),
		agentparams.WithSecurityAgentConfig(securityAgentConfig),
		agentparams.WithSkipAPIKeyInConfig(), // api_key is already provided in the config
	}
	agentOptions = append(agentOptions, secrets.WithWindowsSecretSetupScript(secretResolverPath, true)...)

	// start the agent with that configuration
	v.UpdateEnv(awshost.Provisioner(
		awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
		awshost.WithAgentOptions(agentOptions...),
		awshost.WithAgentClientOptions(
			agentclientparams.WithAuthTokenPath(authTokenFilePath),
			agentclientparams.WithTraceAgentOnPort(apmReceiverPort),
			agentclientparams.WithProcessAgentOnPort(processCmdPort),
		),
	))

	// Currently the framework does not restart the security agent on Windows so we need to do it manually.
	// When the framework will support it, remove the line below and add `agentclientparams.WithSecurityAgentOnPort(securityCmdPort)` to the agent options.
	v.Env().RemoteHost.MustExecute("Restart-Service datadog-security-agent")

	// get auth token
	v.T().Log("Getting the authentication token")
	authtokenContent, err := v.Env().RemoteHost.ReadFile(authTokenFilePath)
	require.NoError(v.T(), err)

	authtoken := strings.TrimSpace(string(authtokenContent))

	// check that the agents are using the first key
	// initially they all resolve it using the secret resolver
	assertAgentsUseKey(v.T(), v.Env().RemoteHost, authtoken, apiKey1)

	// update api_key
	v.T().Log("Updating the api key")
	secretClient.SetSecret("api_key", apiKey2)

	// trigger a refresh of the core-agent secrets
	v.T().Log("Refreshing core-agent secrets")
	secretRefreshOutput := v.Env().Agent.Client.Secret(agentclient.WithArgs([]string{"refresh"}))
	// ensure the api_key was refreshed, fail directly otherwise
	require.Contains(v.T(), secretRefreshOutput, "api_key")

	// and check that the agents are using the new key
	require.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		assertAgentsUseKey(t, v.Env().RemoteHost, authtoken, apiKey2)
	}, 2*configRefreshIntervalSec*time.Second, 1*time.Second)
}
