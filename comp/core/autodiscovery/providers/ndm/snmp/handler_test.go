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
	"init_config": {"namespace": "prod"},
	"instances": [
		{"ip_address": "10.0.0.1", "cred_id": "id-abc"},
		{"ip_address": "10.0.0.2", "cred_id": "id-v3"}
	]
}`

func TestKeyIsSNMP(t *testing.T) {
	assert.Equal(t, "snmp", Key)
	assert.Equal(t, "snmp", newTestHandler(t, twoCredentialsYAML).Key())
}

func TestRenderEmitsOneConfigPerDevice(t *testing.T) {
	h := newTestHandler(t, twoCredentialsYAML)

	configs, err := h.Render("path-a", json.RawMessage(twoInstanceDocument), nil)
	require.NoError(t, err)
	require.Len(t, configs, 2, "one config per device, so a single device can be unscheduled")

	for _, c := range configs {
		assert.Equal(t, "snmp", c.Name)
		assert.Equal(t, "ndm-remote-config:snmp", c.Source)
		require.Len(t, c.Instances, 1)
		assert.YAMLEq(t, "namespace: prod\n", string(c.InitConfig))
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
			"cred_id": "id-abc",
			"port": 1161,
			"timeout_sec": 5,
			"profile": "cisco-nexus",
			"tags": ["site:paris"],
			"interface_configs": [{"match_field": "name", "match_value": "eth0", "in_speed": 25}]
		}]
	}`), nil)

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

	first, err := h.Render("path-a", json.RawMessage(twoInstanceDocument), nil)
	require.NoError(t, err)
	second, err := h.Render("path-a", json.RawMessage(twoInstanceDocument), nil)
	require.NoError(t, err)

	require.Len(t, first, len(second))
	for i := range first {
		assert.Equal(t, first[i].FastDigest(), second[i].FastDigest())
	}
}

func TestRenderOfAnEmptyInstanceListYieldsNothingAndNoError(t *testing.T) {
	h := newTestHandler(t, twoCredentialsYAML)

	configs, err := h.Render("path-a", json.RawMessage(`{"init_config":{},"instances":[]}`), nil)

	assert.NoError(t, err)
	assert.Empty(t, configs)
}

func TestRenderRejectsAValueThatIsNotAnSNMPKey(t *testing.T) {
	h := newTestHandler(t, twoCredentialsYAML)

	configs, err := h.Render("path-a", json.RawMessage(`["not","an","object"]`), nil)

	assert.Error(t, err)
	assert.Empty(t, configs)
}

func TestRenderSchedulesTheResolvableInstancesAndNamesTheRest(t *testing.T) {
	h := newTestHandler(t, twoCredentialsYAML)

	configs, err := h.Render("path-a", json.RawMessage(`{
		"init_config": {},
		"instances": [
			{"ip_address": "10.0.0.1", "cred_id": "id-abc"},
			{"ip_address": "10.0.0.2", "cred_id": "id-missing"},
			{"ip_address": "10.0.0.3", "cred_id": "id-bad-version"},
			{"ip_address": "", "cred_id": "id-abc"}
		]
	}`), nil)

	require.Len(t, configs, 1)
	assert.Contains(t, string(configs[0].Instances[0]), "ip_address: 10.0.0.1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "id-missing", "an unresolved credential is named by the id the document sent")
	assert.Contains(t, err.Error(), "cred-bad-version", "a resolved but unusable credential is named by its own label")
	assert.Contains(t, err.Error(), "10.0.0.2")
	assert.Contains(t, err.Error(), "10.0.0.3")
}

func TestRenderNamesAMissingCredentialByID(t *testing.T) {
	h := newTestHandler(t, twoCredentialsYAML)

	_, err := h.Render("path-a", json.RawMessage(`{"instances":[
		{"ip_address":"10.0.0.1","cred_id": "id-missing"}
	]}`), nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), `"id-missing"`)
}

func TestRenderResolvesOnTheCredentialIDNotItsName(t *testing.T) {
	h := newTestHandler(t, twoCredentialsYAML)

	_, err := h.Render("path-a", json.RawMessage(`{"instances":[
		{"ip_address":"10.0.0.1","cred_id": "cred-abc"}
	]}`), nil)

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

	_, err := h.Render("path-a", json.RawMessage(`{"instances":[{"ip_address":"10.0.0.1","cred_id": "id-bad"}]}`), nil)

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "s3cret-community")
}

func TestRenderErrorIsStableAcrossCalls(t *testing.T) {
	h := newTestHandler(t, twoCredentialsYAML)
	doc := json.RawMessage(`{"instances":[
		{"ip_address":"10.0.0.9","cred_id": "id-z-missing"},
		{"ip_address":"10.0.0.8","cred_id": "id-a-missing"}
	]}`)

	_, first := h.Render("path-a", doc, nil)
	_, second := h.Render("path-a", doc, nil)

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
	doc := json.RawMessage(`{"instances":[{"ip_address":"10.0.0.1","cred_id": "id-abc"}]}`)

	first, err := h.Render("path-a", doc, nil)
	require.NoError(t, err)
	assert.Contains(t, string(first[0].Instances[0]), "community_string: public")

	path := filepath.Join(cfg.GetString("confd_path"), credentialsDir, credentialsFilename)
	require.NoError(t, os.WriteFile(path, []byte("credentials:\n  - id: id-abc\n    name: cred-abc\n    snmp_version: \"2c\"\n    community_string: rotated\n"), 0o600))

	second, err := h.Render("path-a", doc, nil)
	require.NoError(t, err)
	assert.Contains(t, string(second[0].Instances[0]), "community_string: rotated")
}

func TestRenderIgnoresACredentialFileTheCustomerDroppedInTheDirectory(t *testing.T) {
	h := NewHandler(newTestConfig(t, map[string]string{
		credentialsFilename: "credentials:\n  - id: id-abc\n    name: cred-abc\n    snmp_version: \"2c\"\n    community_string: public\n",
		"customer.yaml":     "credentials:\n  - id: id-customer\n    name: customer\n    snmp_version: \"2c\"\n    community_string: public\n",
	}), logmock.New(t))
	doc := `{"instances":[{"ip_address":"10.0.0.1","cred_id": "%s"}]}`

	configs, err := h.Render("path-a", json.RawMessage(fmt.Sprintf(doc, "id-abc")), nil)
	require.NoError(t, err)
	require.Len(t, configs, 1)
	assert.Contains(t, string(configs[0].Instances[0]), "community_string: public")

	_, err = h.Render("path-a", json.RawMessage(fmt.Sprintf(doc, "id-customer")), nil)
	assert.Error(t, err, "a credential from a file the customer dropped in must not resolve")
}

func TestRenderFoldsThePingSectionIntoTheInstanceItNames(t *testing.T) {
	h := newTestHandler(t, twoCredentialsYAML)
	ping := json.RawMessage(`{
		"init_config": {"count": 2, "interval_ms": 10, "timeout_ms": 3000, "linux": {"use_raw_socket": true}},
		"instances": [{"ip_address": "10.0.0.2", "count": 5, "timeout_ms": 1000}]
	}`)

	configs, err := h.Render("path-a", json.RawMessage(twoInstanceDocument), map[string]json.RawMessage{pingKey: ping})
	require.NoError(t, err)
	require.Len(t, configs, 2)

	assert.YAMLEq(t, "namespace: prod\nping:\n  count: 2\n  interval: 10\n  timeout: 3000\n  linux:\n    use_raw_socket: true\n",
		string(configs[0].InitConfig), "the shared ping options reach the check's init config")

	assert.NotContains(t, string(configs[0].Instances[0]), "ping:",
		"10.0.0.1 is not in the ping section, so ping stays off for it")
	assert.YAMLEq(t,
		"ip_address: 10.0.0.2\nsnmp_version: \"3\"\nuser: test-user\nauthProtocol: SHA\nauthKey: test-auth-key\nping:\n  enabled: true\n  count: 5\n  timeout: 1000\n",
		string(configs[1].Instances[0]))
}

func TestRenderEnablesPingOnADeviceThePingSectionOverridesNothingFor(t *testing.T) {
	h := newTestHandler(t, twoCredentialsYAML)
	ping := json.RawMessage(`{"init_config":{"count":4},"instances":[{"ip_address":"10.0.0.1"}]}`)

	configs, err := h.Render("path-a", json.RawMessage(twoInstanceDocument), map[string]json.RawMessage{pingKey: ping})
	require.NoError(t, err)
	require.Len(t, configs, 2)

	assert.Contains(t, string(configs[0].Instances[0]), "ping:\n  enabled: true\n",
		"the instance only enables ping and inherits the shared count")
}

func TestRenderWithNoPingSectionLeavesPingOff(t *testing.T) {
	h := newTestHandler(t, twoCredentialsYAML)

	configs, err := h.Render("path-a", json.RawMessage(twoInstanceDocument), nil)
	require.NoError(t, err)
	require.Len(t, configs, 2)

	for _, c := range configs {
		assert.NotContains(t, string(c.InitConfig), "ping:")
		assert.NotContains(t, string(c.Instances[0]), "ping:")
	}
}

func TestRenderStillSchedulesWhenThePingSectionCannotBeParsed(t *testing.T) {
	h := newTestHandler(t, twoCredentialsYAML)

	configs, err := h.Render("path-a", json.RawMessage(twoInstanceDocument),
		map[string]json.RawMessage{pingKey: json.RawMessage(`["not","an","object"]`)})

	require.NoError(t, err, "an unusable ping section must not stop the snmp instances")
	require.Len(t, configs, 2)
	assert.NotContains(t, string(configs[0].Instances[0]), "ping:")
}

func TestRenderSchedulesTheOtherDevicesWhenThePingSectionNamesAnIPNoInstancePolls(t *testing.T) {
	h := newTestHandler(t, twoCredentialsYAML)
	ping := json.RawMessage(`{"instances":[{"ip_address":"10.0.0.1"},{"ip_address":"192.168.1.1"}]}`)

	configs, err := h.Render("path-a", json.RawMessage(twoInstanceDocument), map[string]json.RawMessage{pingKey: ping})

	require.NoError(t, err, "an unschedulable ping device does not fail the whole document")
	require.Len(t, configs, 2)
	assert.Contains(t, string(configs[0].Instances[0]), "enabled: true")
}

func TestRenderCountsADeviceSkippedForItsCredentialAsPolled(t *testing.T) {
	h := newTestHandler(t, twoCredentialsYAML)

	configs, err := h.Render("path-a", json.RawMessage(`{"instances":[{"ip_address":"10.0.0.1","cred_id":"id-missing"}]}`),
		map[string]json.RawMessage{pingKey: json.RawMessage(`{"instances":[{"ip_address":"10.0.0.1"}]}`)})

	require.Error(t, err, "the credential is still missing")
	assert.Empty(t, configs)
}

func TestHandlerSatisfiesTheHandlerInterface(t *testing.T) {
	var _ handler.Handler = NewHandler(configmock.New(t), logmock.New(t))
}
