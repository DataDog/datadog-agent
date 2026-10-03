// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package snmp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/ndm/handler"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

const twoCredentialsYAML = `
credentials:
  - id: id-abc
    name: cred-abc
    snmp_version: "2c"
    community_string: public
  - id: id-v3
    name: cred-v3
    snmp_version: "3"
    user: test-user
    authProtocol: SHA
    authKey: test-auth-key
  - id: id-bad-version
    name: cred-bad-version
    snmp_version: "9"
    community_string: public
`

func newTestHandler(t *testing.T, credentials string) *Handler {
	t.Helper()
	return NewHandler(newTestConfig(t, map[string]string{credentialsFilename: credentials}), logmock.New(t))
}

const twoInstanceDocument = `{
	"init_config": {"namespace": "prod", "ping": {"enabled": true}},
	"instances": [
		{"ip_address": "10.0.0.1", "cred": {"id": "id-abc", "name": "cred-abc"}},
		{"ip_address": "10.0.0.2", "cred": {"id": "id-v3", "name": "cred-v3"}}
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
		assert.YAMLEq(t, "namespace: prod\nping:\n  enabled: true\n", string(c.InitConfig))
	}

	assert.Contains(t, string(configs[0].Instances[0]), "ip_address: 10.0.0.1")
	assert.Contains(t, string(configs[0].Instances[0]), "community_string: public")
	assert.Contains(t, string(configs[1].Instances[0]), "ip_address: 10.0.0.2")
	assert.Contains(t, string(configs[1].Instances[0]), "user: test-user")
}

func TestRenderCarriesThePerInstanceSettingsToTheCheck(t *testing.T) {
	h := newTestHandler(t, twoCredentialsYAML)

	configs, err := h.Render("path-a", json.RawMessage(`{
		"init_config": {"oid_batch_size": 20, "collect_topology": false},
		"instances": [{
			"ip_address": "10.0.0.1",
			"cred": {"id": "id-abc", "name": "cred-abc"},
			"port": 1161,
			"timeout_sec": 5,
			"profile": "cisco-nexus",
			"tags": ["site:paris"],
			"interface_configs": [{"match_field": "name", "match_value": "eth0", "in_speed": 25}]
		}]
	}`))

	require.NoError(t, err)
	require.Len(t, configs, 1)
	assert.YAMLEq(t, "oid_batch_size: 20\ncollect_topology: false\n", string(configs[0].InitConfig))
	instance := string(configs[0].Instances[0])
	assert.Contains(t, instance, "port: 1161")
	assert.Contains(t, instance, "timeout: 5")
	assert.Contains(t, instance, "profile: cisco-nexus")
	assert.Contains(t, instance, "site:paris")
	assert.Contains(t, instance, "match_value: eth0")
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
			{"ip_address": "10.0.0.1", "cred": {"id": "id-abc", "name": "cred-abc"}},
			{"ip_address": "10.0.0.2", "cred": {"id": "id-missing", "name": "cred-missing"}},
			{"ip_address": "10.0.0.3", "cred": {"id": "id-bad-version", "name": "cred-bad-version"}},
			{"ip_address": "", "cred": {"id": "id-abc", "name": "cred-abc"}}
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

func TestRenderNamesAMissingCredentialByIDAndName(t *testing.T) {
	h := newTestHandler(t, twoCredentialsYAML)

	_, err := h.Render("path-a", json.RawMessage(`{"instances":[
		{"ip_address":"10.0.0.1","cred":{"id":"id-missing","name":"cred-missing"}}
	]}`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), `"cred-missing" (id-missing)`)
}

func TestRenderResolvesOnTheCredentialIDNotItsName(t *testing.T) {
	h := newTestHandler(t, twoCredentialsYAML)

	_, err := h.Render("path-a", json.RawMessage(`{"instances":[
		{"ip_address":"10.0.0.1","cred":{"id":"cred-abc","name":"cred-abc"}}
	]}`))

	require.Error(t, err, "the name must not resolve a credential whose id is id-abc")
}

func TestRenderErrorNamesNoCredentialValue(t *testing.T) {
	h := newTestHandler(t, `
credentials:
  - id: id-bad
    name: cred-bad
    snmp_version: "9"
    community_string: s3cret-community
`)

	_, err := h.Render("path-a", json.RawMessage(`{"instances":[{"ip_address":"10.0.0.1","cred":{"id":"id-bad","name":"cred-bad"}}]}`))

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "s3cret-community")
}

func TestRenderErrorIsStableAcrossCalls(t *testing.T) {
	h := newTestHandler(t, twoCredentialsYAML)
	doc := json.RawMessage(`{"instances":[
		{"ip_address":"10.0.0.9","cred":{"id":"id-z-missing","name":"z-missing"}},
		{"ip_address":"10.0.0.8","cred":{"id":"id-a-missing","name":"a-missing"}}
	]}`)

	_, first := h.Render("path-a", doc)
	_, second := h.Render("path-a", doc)

	require.Error(t, first)
	assert.Equal(t, first.Error(), second.Error())
}

func TestRenderPicksUpACredentialValueThatChangedInPlace(t *testing.T) {
	cfg := newTestConfig(t, map[string]string{credentialsFilename: `
credentials:
  - id: id-abc
    name: cred-abc
    snmp_version: "2c"
    community_string: public
`})
	h := NewHandler(cfg, logmock.New(t))
	doc := json.RawMessage(`{"instances":[{"ip_address":"10.0.0.1","cred":{"id":"id-abc","name":"cred-abc"}}]}`)

	first, err := h.Render("path-a", doc)
	require.NoError(t, err)
	assert.Contains(t, string(first[0].Instances[0]), "community_string: public")

	path := filepath.Join(cfg.GetString("confd_path"), credentialsDir, credentialsFilename)
	require.NoError(t, os.WriteFile(path, []byte("credentials:\n  - id: id-abc\n    name: cred-abc\n    snmp_version: \"2c\"\n    community_string: rotated\n"), 0o600))

	second, err := h.Render("path-a", doc)
	require.NoError(t, err)
	assert.Contains(t, string(second[0].Instances[0]), "community_string: rotated")
}

func TestRenderIgnoresACredentialFileTheCustomerDroppedInTheDirectory(t *testing.T) {
	h := NewHandler(newTestConfig(t, map[string]string{
		credentialsFilename: "credentials:\n  - id: id-abc\n    name: cred-abc\n    snmp_version: \"2c\"\n    community_string: public\n",
		"customer.yaml":     "credentials:\n  - id: id-customer\n    name: customer\n    snmp_version: \"2c\"\n    community_string: public\n",
	}), logmock.New(t))
	doc := `{"instances":[{"ip_address":"10.0.0.1","cred":{"id":"%s","name":"whatever"}}]}`

	configs, err := h.Render("path-a", json.RawMessage(fmt.Sprintf(doc, "id-abc")))
	require.NoError(t, err)
	require.Len(t, configs, 1)
	assert.Contains(t, string(configs[0].Instances[0]), "community_string: public")

	_, err = h.Render("path-a", json.RawMessage(fmt.Sprintf(doc, "id-customer")))
	assert.Error(t, err, "a credential from a file the customer dropped in must not resolve")
}

func TestHandlerSatisfiesTheHandlerInterface(t *testing.T) {
	var _ handler.Handler = NewHandler(configmock.New(t), logmock.New(t))
}
