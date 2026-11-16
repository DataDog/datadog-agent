// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndm

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/ndm/handler"
	ndmsnmp "github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/ndm/snmp"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// backendDocument is the payload the backend writes: one document per Agent.
// Ping arrives in its own key, naming the devices it covers.
const backendDocument = `{
	"ping": {
		"init_config": {"count": 2, "timeout_ms": 3000},
		"instances": [{"ip_address": "10.0.0.1"}]
	},
	"snmp": {
		"init_config": {"namespace": "prod"},
		"instances": [
			{"ip_address": "10.0.0.1", "cred_id": "id-abc"},
			{"ip_address": "10.0.0.2", "cred_id": "id-abc"}
		]
	}
}`

// newTestConfig returns a configuration whose conf.d/snmp.d/credentials holds
// the single credential the documents below reference.
func newTestConfig(t *testing.T) model.BuildableConfig {
	t.Helper()
	cfg := configmock.New(t)
	confd := t.TempDir()
	dir := filepath.Join(confd, "snmp.d", "credentials")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "snmp_credentials.yaml"), []byte(`
credentials:
  - id: id-abc
    name: cred-abc
    snmp_version: "2c"
    community_string: public
`), 0o600))
	cfg.SetInTest("confd_path", confd)
	return cfg
}

func TestABackendDocumentBecomesSchedulableSNMPChecks(t *testing.T) {
	cfg := newTestConfig(t)
	logComp := logmock.New(t)
	p, err := NewProvider(logComp, []handler.Handler{ndmsnmp.NewHandler(cfg, logComp)})
	require.NoError(t, err)

	ch := p.Stream(context.Background())
	rec := newRecorder()

	p.Update(map[string]state.RawConfig{
		"datadog/2/NDM_CONFIG/ndm-1/config":   rawConfig(backendDocument),
		"datadog/2/NDM_CONFIG/empty-1/config": rawConfig(`{}`),
		"datadog/2/NDM_CONFIG/other-1/config": rawConfig(`{"autodiscovery":{"configs":[]}}`),
	}, rec.callback)

	changes := drain(t, ch)
	require.Len(t, changes, 1)
	require.Len(t, changes[0].Schedule, 2, "one check config per device")

	for _, c := range changes[0].Schedule {
		assert.Equal(t, "snmp", c.Name)
		assert.Equal(t, "ndm-remote-config:snmp", c.Source)
		assert.YAMLEq(t, "namespace: prod\nping:\n  count: 2\n  timeout: 3000\n", string(c.InitConfig))
		require.Len(t, c.Instances, 1)
		assert.Contains(t, string(c.Instances[0]), "community_string: public")
	}
	assert.Contains(t, string(changes[0].Schedule[0].Instances[0]), "ip_address: 10.0.0.1")
	assert.Contains(t, string(changes[0].Schedule[0].Instances[0]), "enabled: true",
		"the device the ping key names has ping enabled on its instance")
	assert.Contains(t, string(changes[0].Schedule[1].Instances[0]), "ip_address: 10.0.0.2")
	assert.NotContains(t, string(changes[0].Schedule[1].Instances[0]), "ping:",
		"a device the ping key omits keeps the check's own default of disabled")

	assert.Len(t, rec.states, 1)
	assert.Equal(t, state.ApplyStateAcknowledged,
		rec.states["datadog/2/NDM_CONFIG/ndm-1/config"].State)
	assert.Empty(t, p.GetConfigErrors())
}

func TestADocumentWithAMissingCredentialSchedulesTheRestAndReportsAnError(t *testing.T) {
	cfg := newTestConfig(t)
	logComp := logmock.New(t)
	p, err := NewProvider(logComp, []handler.Handler{ndmsnmp.NewHandler(cfg, logComp)})
	require.NoError(t, err)

	ch := p.Stream(context.Background())
	rec := newRecorder()
	const path = "datadog/2/NDM_CONFIG/ndm-1/config"

	p.Update(map[string]state.RawConfig{path: rawConfig(`{"snmp":{"instances":[
		{"ip_address":"10.0.0.1","cred_id":"id-abc"},
		{"ip_address":"10.0.0.2","cred_id":"id-not-delivered-yet"}
	]}}`)}, rec.callback)

	changes := drain(t, ch)
	require.Len(t, changes, 1)
	assert.Len(t, changes[0].Schedule, 1, "the resolvable device still collects")

	assert.Equal(t, state.ApplyStateError, rec.states[path].State)
	assert.Contains(t, rec.states[path].Error, "snmp: ")
	assert.Contains(t, rec.states[path].Error, "id-not-delivered-yet")

	errs := p.GetConfigErrors()
	require.Contains(t, errs, path)
	for message := range errs[path] {
		assert.Contains(t, message, "snmp: ")
	}
}

func TestRemovingTheDocumentUnschedulesEveryDevice(t *testing.T) {
	cfg := newTestConfig(t)
	logComp := logmock.New(t)
	p, err := NewProvider(logComp, []handler.Handler{ndmsnmp.NewHandler(cfg, logComp)})
	require.NoError(t, err)

	ch := p.Stream(context.Background())
	rec := newRecorder()
	const path = "datadog/2/NDM_CONFIG/ndm-1/config"

	p.Update(map[string]state.RawConfig{path: rawConfig(backendDocument)}, rec.callback)
	require.Len(t, drain(t, ch), 1)

	p.Update(map[string]state.RawConfig{}, rec.callback)

	changes := drain(t, ch)
	require.Len(t, changes, 1)
	assert.Len(t, changes[0].Unschedule, 2)
	assert.Empty(t, changes[0].Schedule)
}

func TestARepeatedIdenticalDocumentEmitsNothing(t *testing.T) {
	cfg := newTestConfig(t)
	logComp := logmock.New(t)
	p, err := NewProvider(logComp, []handler.Handler{ndmsnmp.NewHandler(cfg, logComp)})
	require.NoError(t, err)

	ch := p.Stream(context.Background())
	rec := newRecorder()
	const path = "datadog/2/NDM_CONFIG/ndm-1/config"

	p.Update(map[string]state.RawConfig{path: rawConfig(backendDocument)}, rec.callback)
	require.Len(t, drain(t, ch), 1, "the first delivery schedules the two devices")

	p.Update(map[string]state.RawConfig{path: rawConfig(backendDocument)}, rec.callback)

	assert.Empty(t, drain(t, ch), "an identical document must emit no schedule or unschedule churn")
	assert.Equal(t, state.ApplyStateAcknowledged, rec.states[path].State)
}

func TestADocumentWhoseInstancesAreAllUnresolvableSchedulesNothingAndErrors(t *testing.T) {
	cfg := newTestConfig(t)
	logComp := logmock.New(t)
	p, err := NewProvider(logComp, []handler.Handler{ndmsnmp.NewHandler(cfg, logComp)})
	require.NoError(t, err)

	ch := p.Stream(context.Background())
	rec := newRecorder()
	const path = "datadog/2/NDM_CONFIG/ndm-1/config"

	p.Update(map[string]state.RawConfig{path: rawConfig(`{"snmp":{"instances":[
		{"ip_address":"10.0.0.1","cred_id":"id-unknown-1"},
		{"ip_address":"10.0.0.2","cred_id":"id-unknown-2"}
	]}}`)}, rec.callback)

	changes := drain(t, ch)
	assert.Empty(t, changes, "nothing is scheduled when every instance is unresolvable")

	assert.Equal(t, state.ApplyStateError, rec.states[path].State)
	assert.Contains(t, rec.states[path].Error, "snmp: ")
	assert.Contains(t, rec.states[path].Error, "id-unknown-1")
	assert.Contains(t, rec.states[path].Error, "id-unknown-2")

	errs := p.GetConfigErrors()
	require.Contains(t, errs, path)
}
