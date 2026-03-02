// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configrefresh

import (
	"bytes"
	_ "embed"
	"html/template"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

const (
	AgentIpcUseSocket        = false
	AgentIpcPort             = 5004
	SecurityCmdPort          = 5010
	ApmCmdPort               = 5012
	ApmReceiverPort          = 8126
	ProcessCmdPort           = 6162
	ConfigRefreshIntervalSec = 10
)

//go:embed fixtures/config.yaml.tmpl
var CoreConfigTmpl string

//go:embed fixtures/security-agent.yaml
var SecurityAgentConfig string

var (
	APIKey1 = strings.Repeat("1", 32)
	APIKey2 = strings.Repeat("2", 32)
)

// AssertAgentsUseKey checks that all agents are using the given key.
func AssertAgentsUseKey(t assert.TestingT, host *components.RemoteHost, authtoken, key string) {
	if h, ok := t.(testing.TB); ok {
		h.Helper()
	}

	hostHTTPClient := host.NewHTTPClient()
	for _, endpoint := range []AgentConfigEndpointInfo{
		TraceConfigEndpoint(ApmCmdPort),
		ProcessConfigEndpoint(ProcessCmdPort),
		SecurityConfigEndpoint(SecurityCmdPort),
	} {
		req, err := endpoint.HTTPRequest(authtoken)
		if !assert.NoErrorf(t, err, "failed to create request for %s", endpoint.Name) {
			continue
		}

		resp, err := hostHTTPClient.Do(req)
		if !assert.NoErrorf(t, err, "failed to fetch config from %s", endpoint.Name) {
			continue
		}
		defer resp.Body.Close()

		if !assert.Equalf(t, http.StatusOK, resp.StatusCode, "unexpected status code for %s", endpoint.Name) {
			continue
		}

		cfg, err := io.ReadAll(resp.Body)
		if !assert.NoErrorf(t, err, "failed to read response body from %s", endpoint.Name) {
			continue
		}

		AssertConfigHasKey(t, string(cfg), key, "checking key used by "+endpoint.Name)
	}
}

// AssertConfigHasKey checks that configYAML contains the given key.
// As the config is scrubbed, it only checks the last 5 characters of the keys.
func AssertConfigHasKey(t assert.TestingT, configYAML, key string, context string) {
	if h, ok := t.(testing.TB); ok {
		h.Helper()
	}

	var cfg map[string]interface{}
	err := yaml.Unmarshal([]byte(configYAML), &cfg)
	if !assert.NoError(t, err, "failed to unmarshal config: '%v'", configYAML) {
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

// FillTmplConfig fills the template with the given variables and returns the result.
func FillTmplConfig(t *testing.T, tmplContent string, templateVars any) string {
	t.Helper()

	var buffer bytes.Buffer

	tmpl, err := template.New("").Parse(tmplContent)
	require.NoError(t, err)

	err = tmpl.Execute(&buffer, templateVars)
	require.NoError(t, err)

	return buffer.String()
}
