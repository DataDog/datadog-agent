// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package snmp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/ndm/handler"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
)

const twoCredentialsYAML = `
init_config:
instances:
  - tags:
    - credential-name:cred-abc
    snmp_version: "2c"
    community_string: public
  - tags:
    - credential-name:cred-v3
    snmp_version: "3"
    user: test-user
    authProtocol: SHA
    authKey: test-auth-key
  - tags:
    - credential-name:cred-bad-version
    snmp_version: "9"
    community_string: public
`

func newTestHandler(t *testing.T, credentials string) *Handler {
	t.Helper()
	return NewHandler(newTestConfig(t, credentials), logmock.New(t))
}

const twoInstanceDocument = `{
	"init_config": {"loader": "core", "ping": {"enabled": true}},
	"instances": [
		{"ip_address": "10.0.0.1", "cred_id": "cred-abc"},
		{"ip_address": "10.0.0.2", "cred_id": "cred-v3"}
	]
}`

func TestKeyIsSNMP(t *testing.T) {
	assert.Equal(t, "snmp", Key)
	assert.Equal(t, "snmp", newTestHandler(t, twoCredentialsYAML).Key())
}

func TestRenderEmitsOneConfigPerDevice(t *testing.T) {
	h := newTestHandler(t, twoCredentialsYAML)

	configs, err := h.Render("path-a", json.RawMessage(twoInstanceDocument))
	require.NoError(t, err)
	require.Len(t, configs, 2, "one config per device, so a single device can be unscheduled")

	for _, c := range configs {
		assert.Equal(t, "snmp", c.Name)
		assert.Equal(t, "ndm-remote-config:snmp", c.Source)
		require.Len(t, c.Instances, 1)
		assert.YAMLEq(t, "loader: core\nping:\n  enabled: true\n", string(c.InitConfig))
	}

	assert.Contains(t, string(configs[0].Instances[0]), "ip_address: 10.0.0.1")
	assert.Contains(t, string(configs[0].Instances[0]), "community_string: public")
	assert.Contains(t, string(configs[1].Instances[0]), "ip_address: 10.0.0.2")
	assert.Contains(t, string(configs[1].Instances[0]), "user: test-user")
}

func TestRenderKeepsTheDocumentOrder(t *testing.T) {
	h := newTestHandler(t, twoCredentialsYAML)

	first, err := h.Render("path-a", json.RawMessage(twoInstanceDocument))
	require.NoError(t, err)
	second, err := h.Render("path-a", json.RawMessage(twoInstanceDocument))
	require.NoError(t, err)

	require.Len(t, first, len(second))
	for i := range first {
		assert.Equal(t, first[i].FastDigest(), second[i].FastDigest())
	}
}

func TestRenderOfAnEmptyInstanceListYieldsNothingAndNoError(t *testing.T) {
	h := newTestHandler(t, twoCredentialsYAML)

	configs, err := h.Render("path-a", json.RawMessage(`{"init_config":{},"instances":[]}`))

	assert.NoError(t, err)
	assert.Empty(t, configs)
}

func TestRenderRejectsAValueThatIsNotAnSNMPKey(t *testing.T) {
	h := newTestHandler(t, twoCredentialsYAML)

	configs, err := h.Render("path-a", json.RawMessage(`["not","an","object"]`))

	assert.Error(t, err)
	assert.Empty(t, configs)
}

func TestRenderSchedulesTheResolvableInstancesAndNamesTheRest(t *testing.T) {
	h := newTestHandler(t, twoCredentialsYAML)

	configs, err := h.Render("path-a", json.RawMessage(`{
		"init_config": {},
		"instances": [
			{"ip_address": "10.0.0.1", "cred_id": "cred-abc"},
			{"ip_address": "10.0.0.2", "cred_id": "cred-missing"},
			{"ip_address": "10.0.0.3", "cred_id": "cred-bad-version"},
			{"ip_address": "", "cred_id": "cred-abc"}
		]
	}`))

	require.Len(t, configs, 1)
	assert.Contains(t, string(configs[0].Instances[0]), "ip_address: 10.0.0.1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cred-missing")
	assert.Contains(t, err.Error(), "cred-bad-version")
	assert.Contains(t, err.Error(), "10.0.0.2")
	assert.Contains(t, err.Error(), "10.0.0.3")
}

func TestRenderErrorNamesNoCredentialValue(t *testing.T) {
	h := newTestHandler(t, `
instances:
  - tags:
    - credential-name:cred-bad
    snmp_version: "9"
    community_string: s3cret-community
`)

	_, err := h.Render("path-a", json.RawMessage(`{"instances":[{"ip_address":"10.0.0.1","cred_id":"cred-bad"}]}`))

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "s3cret-community")
}

func TestRenderErrorIsStableAcrossCalls(t *testing.T) {
	h := newTestHandler(t, twoCredentialsYAML)
	doc := json.RawMessage(`{"instances":[
		{"ip_address":"10.0.0.9","cred_id":"z-missing"},
		{"ip_address":"10.0.0.8","cred_id":"a-missing"}
	]}`)

	_, first := h.Render("path-a", doc)
	_, second := h.Render("path-a", doc)

	require.Error(t, first)
	assert.Equal(t, first.Error(), second.Error())
}

func TestRenderPicksUpACredentialValueThatChangedInPlace(t *testing.T) {
	cfg := newTestConfig(t, `
instances:
  - tags:
    - credential-name:cred-abc
    snmp_version: "2c"
    community_string: public
`)
	h := NewHandler(cfg, logmock.New(t))
	doc := json.RawMessage(`{"instances":[{"ip_address":"10.0.0.1","cred_id":"cred-abc"}]}`)

	first, err := h.Render("path-a", doc)
	require.NoError(t, err)
	assert.Contains(t, string(first[0].Instances[0]), "community_string: public")

	writeCredentials(t, cfg.GetString("confd_path"), `
instances:
  - tags:
    - credential-name:cred-abc
    snmp_version: "2c"
    community_string: rotated
`)

	second, err := h.Render("path-a", doc)
	require.NoError(t, err)
	assert.Contains(t, string(second[0].Instances[0]), "community_string: rotated")
}

func TestHandlerSatisfiesTheHandlerInterface(t *testing.T) {
	var _ handler.Handler = NewHandler(newTestConfig(t, ""), logmock.New(t))
}
