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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"
	secrets "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-shared-components/secretsutils"
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

	secretClient := secrets.NewSecretClient(v.T(), v.Env().RemoteHost, rootDir)
	secretClient.SetSecret("api_key", apiKey1)

	// fill the config template
	templateVars := map[string]interface{}{
		"AuthTokenFilePath":        authTokenFilePath,
		"SecretDirectory":          rootDir,
		"SecretResolver":           secretResolverPath,
		"ConfigRefreshIntervalSec": configRefreshIntervalSec,
		"ApmCmdPort":               apmCmdPort,
		"ProcessCmdPort":           processCmdPort,
		"SecurityCmdPort":          securityCmdPort,
		"AgentIpcPort":             agentIpcPort,
		"SecretBackendCommandAllowGroupExecPermOption": "true",
	}
	coreconfig := fillTmplConfig(v.T(), coreConfigTmpl, templateVars)

	// start the agent with that configuration
	v.UpdateEnv(awshost.Provisioner(
		awshost.WithAgentOptions(
			secrets.WithUnixSecretSetupScript(secretResolverPath, true),
			agentparams.WithAgentConfig(coreconfig),
			agentparams.WithSecurityAgentConfig(securityAgentConfig),
			agentparams.WithSkipAPIKeyInConfig(), // api_key is already provided in the config
		),
		awshost.WithAgentClientOptions(
			agentclientparams.WithAuthTokenPath(authTokenFilePath),
			agentclientparams.WithTraceAgentOnPort(apmReceiverPort),
			agentclientparams.WithProcessAgentOnPort(processCmdPort),
			agentclientparams.WithSecurityAgentOnPort(securityCmdPort),
		),
	))

	// get auth token
	v.T().Log("Getting the authentication token")
	authtokenContent := v.Env().RemoteHost.MustExecute("sudo cat " + authTokenFilePath)
	authtoken := strings.TrimSpace(authtokenContent)

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
