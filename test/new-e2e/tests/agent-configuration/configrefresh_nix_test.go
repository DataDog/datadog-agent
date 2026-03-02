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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclientparams"
	configrefresh "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-configuration/config-refresh"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-configuration/secretsutils"
)

type configRefreshLinuxSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestConfigRefreshLinuxSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &configRefreshLinuxSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
}

func (v *configRefreshLinuxSuite) TestConfigRefresh() {
	rootDir := "/tmp/" + v.T().Name()
	v.Env().RemoteHost.MkdirAll(rootDir)

	authTokenFilePath := "/etc/datadog-agent/auth_token"
	secretResolverPath := filepath.Join(rootDir, "secret-resolver.py")

	v.T().Log("Setting up the secret resolver and the initial api key file")

	secretClient := secretsutils.NewClient(v.T(), v.Env().RemoteHost, rootDir)
	secretClient.SetSecret("api_key", configrefresh.APIKey1)

	// fill the config template
	templateVars := map[string]interface{}{
		"AuthTokenFilePath":        authTokenFilePath,
		"SecretDirectory":          rootDir,
		"SecretResolver":           secretResolverPath,
		"ConfigRefreshIntervalSec": configrefresh.ConfigRefreshIntervalSec,
		"ApmCmdPort":               configrefresh.ApmCmdPort,
		"ProcessCmdPort":           configrefresh.ProcessCmdPort,
		"SecurityCmdPort":          configrefresh.SecurityCmdPort,
		"AgentIpcUseSocket":        configrefresh.AgentIpcUseSocket, // defaults to false
		"AgentIpcPort":             configrefresh.AgentIpcPort,
		"SecretBackendCommandAllowGroupExecPermOption": "true",
	}
	coreconfig := configrefresh.FillTmplConfig(v.T(), configrefresh.CoreConfigTmpl, templateVars)

	// start the agent with that configuration
	v.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(scenec2.WithAgentOptions(
			secretsutils.WithUnixSetupScript(secretResolverPath, true),
			agentparams.WithAgentConfig(coreconfig),
			agentparams.WithSecurityAgentConfig(configrefresh.SecurityAgentConfig),
			agentparams.WithSkipAPIKeyInConfig(), // api_key is already provided in the config
		)),
		awshost.WithRunOptions(scenec2.WithAgentClientOptions(
			agentclientparams.WithAuthTokenPath(authTokenFilePath),
			agentclientparams.WithTraceAgentOnPort(configrefresh.ApmReceiverPort),
			agentclientparams.WithProcessAgentOnPort(configrefresh.ProcessCmdPort),
			agentclientparams.WithSecurityAgentOnPort(configrefresh.SecurityCmdPort),
		)),
	))

	// get auth token
	v.T().Log("Getting the authentication token")
	authtokenContent := v.Env().RemoteHost.MustExecute("sudo cat " + authTokenFilePath)
	authtoken := strings.TrimSpace(authtokenContent)

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

func (v *configRefreshLinuxSuite) TestConfigRefreshOverSocket() {
	rootDir := "/tmp/" + v.T().Name()
	v.Env().RemoteHost.MkdirAll(rootDir)

	authTokenFilePath := "/etc/datadog-agent/auth_token"
	secretResolverPath := filepath.Join(rootDir, "secret-resolver.py")

	v.T().Log("Setting up the secret resolver and the initial api key file")

	secretClient := secretsutils.NewClient(v.T(), v.Env().RemoteHost, rootDir)
	secretClient.SetSecret("api_key", configrefresh.APIKey1)

	// fill the config template
	templateVars := map[string]interface{}{
		"AuthTokenFilePath":        authTokenFilePath,
		"SecretDirectory":          rootDir,
		"SecretResolver":           secretResolverPath,
		"ConfigRefreshIntervalSec": configrefresh.ConfigRefreshIntervalSec,
		"ApmCmdPort":               configrefresh.ApmCmdPort,
		"ProcessCmdPort":           configrefresh.ProcessCmdPort,
		"SecurityCmdPort":          configrefresh.SecurityCmdPort,
		"AgentIpcUseSocket":        true,
		"AgentIpcPort":             0,
		"SecretBackendCommandAllowGroupExecPermOption": "true",
	}
	coreconfig := configrefresh.FillTmplConfig(v.T(), configrefresh.CoreConfigTmpl, templateVars)

	// start the agent with that configuration
	v.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(scenec2.WithAgentOptions(
			secretsutils.WithUnixSetupScript(secretResolverPath, true),
			agentparams.WithAgentConfig(coreconfig),
			agentparams.WithSecurityAgentConfig(configrefresh.SecurityAgentConfig),
			agentparams.WithSkipAPIKeyInConfig(), // api_key is already provided in the config
		)),
		awshost.WithRunOptions(scenec2.WithAgentClientOptions(
			agentclientparams.WithAuthTokenPath(authTokenFilePath),
			agentclientparams.WithTraceAgentOnPort(configrefresh.ApmReceiverPort),
			agentclientparams.WithProcessAgentOnPort(configrefresh.ProcessCmdPort),
			agentclientparams.WithSecurityAgentOnPort(configrefresh.SecurityCmdPort),
		)),
	))

	// get auth token
	v.T().Log("Getting the authentication token")
	authtokenContent := v.Env().RemoteHost.MustExecute("sudo cat " + authTokenFilePath)
	authtoken := strings.TrimSpace(authtokenContent)

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
