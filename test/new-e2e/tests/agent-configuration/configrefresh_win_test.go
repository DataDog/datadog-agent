// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentconfiguration

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclientparams"
	configrefresh "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-configuration/config-refresh"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-configuration/secretsutils"
)

type configRefreshWindowsSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestConfigRefreshWindowsSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &configRefreshWindowsSuite{}, e2e.WithProvisioner(awshost.Provisioner(awshost.WithRunOptions(ec2.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault))))))
}

func (v *configRefreshWindowsSuite) TestConfigRefresh() {
	rootDir := "C:/tmp/" + v.T().Name()
	v.Env().RemoteHost.MkdirAll(rootDir)

	authTokenFilePath := `C:\ProgramData\Datadog\auth_token`
	secretResolverPath := filepath.Join(rootDir, "wrapper.bat")

	v.T().Log("Setting up the secret resolver and the initial api key file")

	secretClient := secretsutils.NewClient(v.T(), v.Env().RemoteHost, rootDir)
	secretClient.SetSecret("api_key", configrefresh.APIKey1)

	templateVars := map[string]interface{}{
		"AuthTokenFilePath":        authTokenFilePath,
		"SecretDirectory":          rootDir,
		"SecretResolver":           secretResolverPath,
		"ConfigRefreshIntervalSec": configrefresh.ConfigRefreshIntervalSec,
		"ApmCmdPort":               configrefresh.ApmCmdPort,
		"ProcessCmdPort":           configrefresh.ProcessCmdPort,
		"SecurityCmdPort":          configrefresh.SecurityCmdPort,
		"AgentIpcUseSocket":        configrefresh.AgentIpcUseSocket, // NamedPipe is not implemented yet for windows
		"AgentIpcPort":             configrefresh.AgentIpcPort,
		"SecretBackendCommandAllowGroupExecPermOption": "false", // this is not supported on Windows
	}
	coreconfig := configrefresh.FillTmplConfig(v.T(), configrefresh.CoreConfigTmpl, templateVars)

	agentOptions := []func(*agentparams.Params) error{
		agentparams.WithAgentConfig(coreconfig),
		agentparams.WithSecurityAgentConfig(configrefresh.SecurityAgentConfig),
		agentparams.WithSkipAPIKeyInConfig(), // api_key is already provided in the config
	}
	agentOptions = append(agentOptions, secretsutils.WithWindowsSetupScript(secretResolverPath, true)...)

	// start the agent with that configuration
	v.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(ec2.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault)),
			ec2.WithAgentOptions(agentOptions...),
			ec2.WithAgentClientOptions(
				agentclientparams.WithAuthTokenPath(authTokenFilePath),
				agentclientparams.WithTraceAgentOnPort(configrefresh.ApmReceiverPort),
				agentclientparams.WithProcessAgentOnPort(configrefresh.ProcessCmdPort),
			)),
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
	configrefresh.AssertAgentsUseKey(v.T(), v.Env().RemoteHost, authtoken, configrefresh.APIKey1)

	// update api_key
	v.T().Log("Updating the api key")
	secretClient.SetSecret("api_key", configrefresh.APIKey2)

	// trigger a refresh of the core-agent secrets
	v.T().Log("Refreshing core-agent secrets")
	secretRefreshOutput := v.Env().Agent.Client.Secret(agentclient.WithArgs([]string{"refresh"}))
	// ensure the api_key was refreshed, fail directly otherwise
	require.Contains(v.T(), secretRefreshOutput, "api_key")

	// and check that the agents are using the new key
	require.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		configrefresh.AssertAgentsUseKey(t, v.Env().RemoteHost, authtoken, configrefresh.APIKey2)
	}, 2*configrefresh.ConfigRefreshIntervalSec*time.Second, 1*time.Second)
}
