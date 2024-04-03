// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configrefresh

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

const (
	agentIpcPort             = 5004
	securityCmdPort          = 5010
	apmCmdPort               = 5012
	processCmdPort           = 6162
	configRefreshIntervalSec = 10
)

//go:embed fixtures/config.yaml.tmpl
var coreConfigTmpl string

//go:embed fixtures/security-agent.yaml
var securityAgentConfig string

//go:embed fixtures/secret-resolver.py
var secretResolverScript []byte

var (
	apiKey1 = strings.Repeat("1", 32)
	apiKey2 = strings.Repeat("2", 32)
)

type configRefreshSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestConfigRefreshSuite(t *testing.T) {
	e2e.Run(t, &configRefreshSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
}

func (v *configRefreshSuite) TestConfigRefresh() {
	rootDir := "/tmp/" + v.T().Name()
	v.Env().RemoteHost.MkdirAll(rootDir)

	authTokenFilePath := "/etc/datadog-agent/auth_token"
	secretResolverPath := filepath.Join(rootDir, "secret-resolver.py")
	apiKeyFile := filepath.Join(rootDir, "api_key")

	v.T().Log("Setting up the secret resolver and the initial api key file")

	secretResolverOptions := fileOptions{usergroup: "dd-agent:root", perm: "750", content: secretResolverScript}
	createFile(v.T(), v.Env().RemoteHost, secretResolverPath, secretResolverOptions)
	createFile(v.T(), v.Env().RemoteHost, apiKeyFile, fileOptions{content: []byte(apiKey1)})

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
	}
	coreconfig := fillTmplConfig(v.T(), coreConfigTmpl, templateVars)

	// start the agent with that configuration
	v.UpdateEnv(awshost.Provisioner(
		awshost.WithAgentOptions(
			agentparams.WithAgentConfig(coreconfig),
			agentparams.WithSecurityAgentConfig(securityAgentConfig),
			agentparams.WithSkipAPIKeyInConfig(), // api_key is already provided in the config
		),
	))

	// get auth token
	v.T().Log("Getting the authentication token")
	authtokenContent := v.Env().RemoteHost.MustExecute("sudo cat " + authTokenFilePath)
	authtoken := strings.TrimSpace(authtokenContent)

	// check that the agents are using the first key
	// initially they all resolve it using the secret resolver
	//
	// we have to use an Eventually here because the test can start before the non-core agents are ready
	require.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		assertAgentsUseKey(t, v.Env().RemoteHost, authtoken, apiKey1)
	}, 2*time.Minute, 10*time.Second)

	// update api_key
	v.T().Log("Updating the api key")
	v.Env().RemoteHost.WriteFile(apiKeyFile, []byte(apiKey2))

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

type fileOptions struct {
	usergroup string
	content   []byte
	perm      string
}

func createFile(t *testing.T, host *components.RemoteHost, filepath string, options fileOptions) {
	t.Logf("File '%s': creating", filepath)

	// remove it if it exists
	host.MustExecute("rm -f " + filepath)

	_, err := host.WriteFile(filepath, options.content)
	require.NoError(t, err)

	if options.perm != "" {
		t.Logf("File '%s': setting permissions %s", filepath, options.perm)
		host.MustExecute(fmt.Sprintf("sudo chmod %s %s", options.perm, filepath))
	}

	if options.usergroup != "" {
		t.Logf("File '%s': setting user:group %s", filepath, options.usergroup)
		host.MustExecute(fmt.Sprintf("sudo chown %s %s", options.usergroup, filepath))
	}
}

// assertAgentsUseKey checks that all agents are using the given key.
func assertAgentsUseKey(t assert.TestingT, host *components.RemoteHost, authtoken, key string) {
	if h, ok := t.(testing.TB); ok {
		h.Helper()
	}

	for _, endpoint := range []agentConfigEndpointInfo{
		traceConfigEndpoint(apmCmdPort),
		processConfigEndpoint(processCmdPort),
		securityConfigEndpoint(securityCmdPort),
	} {
		cmd := endpoint.fetchCommand(authtoken)
		cfg, err := host.Execute(cmd)
		if assert.NoErrorf(t, err, "failed to fetch config from %s using cmd: %s", endpoint.name, cmd) {
			assertConfigHasKey(t, cfg, key, "checking key used by "+endpoint.name)
		}
	}
}

// assertConfigHasKey checks that configYAML contains the given key.
// As the config is scrubbed, it only checks the last 5 characters of the keys.
func assertConfigHasKey(t assert.TestingT, configYAML, key string, context string) {
	if h, ok := t.(testing.TB); ok {
		h.Helper()
	}

	var cfg map[string]interface{}
	err := yaml.Unmarshal([]byte(configYAML), &cfg)
	if !assert.NoError(t, err, context) {
		return
	}

	if !assert.Contains(t, cfg, "api_key", context) {
		return
	}

	keyEnd := key[len(key)-5:]
	actual := cfg["api_key"].(string)
	actualEnd := actual[len(actual)-5:]

	assert.Equal(t, keyEnd, actualEnd, context)
}

// fillTmplConfig fills the template with the given variables and returns the result.
func fillTmplConfig(t *testing.T, tmplContent string, templateVars any) string {
	t.Helper()

	var buffer bytes.Buffer

	tmpl, err := template.New("").Parse(tmplContent)
	require.NoError(t, err)

	err = tmpl.Execute(&buffer, templateVars)
	require.NoError(t, err)

	return buffer.String()
}
