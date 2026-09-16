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
const backendDocument = `{
	"snmp": {
		"init_config": {"loader": "core", "ping": {"enabled": true}},
		"instances": [
			{"ip_address": "10.0.0.1", "cred_id": "cred-abc"},
			{"ip_address": "10.0.0.2", "cred_id": "cred-abc"}
		]
	}
}`

func TestABackendDocumentBecomesSchedulableSNMPChecks(t *testing.T) {
	cfg := credentialsConfig(t)
	logComp := logmock.New(t)
	p, err := NewProvider(logComp, []handler.Handler{ndmsnmp.NewHandler(cfg, logComp)})
	require.NoError(t, err)

	ch := p.Stream(context.Background())
	rec := newRecorder()

	p.Update(map[string]state.RawConfig{
		"datadog/2/MANAGED_DEPLOYMENTS_DEBUG/ndm-1/config":   rawConfig(backendDocument),
		"datadog/2/MANAGED_DEPLOYMENTS_DEBUG/empty-1/config": rawConfig(`{}`),
		"datadog/2/MANAGED_DEPLOYMENTS_DEBUG/other-1/config": rawConfig(`{"debug-config-pct":10}`),
	}, rec.callback)

	changes := drain(t, ch)
	require.Len(t, changes, 1)
	require.Len(t, changes[0].Schedule, 2, "one check config per device")

	for _, c := range changes[0].Schedule {
		assert.Equal(t, "snmp", c.Name)
		assert.Equal(t, "ndm-remote-config:snmp", c.Source)
		assert.YAMLEq(t, "loader: core\nping:\n  enabled: true\n", string(c.InitConfig))
		require.Len(t, c.Instances, 1)
		assert.Contains(t, string(c.Instances[0]), "community_string: public")
	}
	assert.Contains(t, string(changes[0].Schedule[0].Instances[0]), "ip_address: 10.0.0.1")
	assert.Contains(t, string(changes[0].Schedule[1].Instances[0]), "ip_address: 10.0.0.2")

	assert.Len(t, rec.states, 1)
	assert.Equal(t, state.ApplyStateAcknowledged,
		rec.states["datadog/2/MANAGED_DEPLOYMENTS_DEBUG/ndm-1/config"].State)
	assert.Empty(t, p.GetConfigErrors())
}

func TestADocumentWithAMissingCredentialSchedulesTheRestAndReportsAnError(t *testing.T) {
	cfg := credentialsConfig(t)
	logComp := logmock.New(t)
	p, err := NewProvider(logComp, []handler.Handler{ndmsnmp.NewHandler(cfg, logComp)})
	require.NoError(t, err)

	ch := p.Stream(context.Background())
	rec := newRecorder()
	const path = "datadog/2/MANAGED_DEPLOYMENTS_DEBUG/ndm-1/config"

	p.Update(map[string]state.RawConfig{path: rawConfig(`{"snmp":{"instances":[
		{"ip_address":"10.0.0.1","cred_id":"cred-abc"},
		{"ip_address":"10.0.0.2","cred_id":"cred-not-delivered-yet"}
	]}}`)}, rec.callback)

	changes := drain(t, ch)
	require.Len(t, changes, 1)
	assert.Len(t, changes[0].Schedule, 1, "the resolvable device still collects")

	assert.Equal(t, state.ApplyStateError, rec.states[path].State)
	assert.Contains(t, rec.states[path].Error, "snmp: ")
	assert.Contains(t, rec.states[path].Error, "cred-not-delivered-yet")

	errs := p.GetConfigErrors()
	require.Contains(t, errs, path)
	for message := range errs[path] {
		assert.Contains(t, message, "snmp: ")
	}
}

func TestRemovingTheDocumentUnschedulesEveryDevice(t *testing.T) {
	cfg := credentialsConfig(t)
	logComp := logmock.New(t)
	p, err := NewProvider(logComp, []handler.Handler{ndmsnmp.NewHandler(cfg, logComp)})
	require.NoError(t, err)

	ch := p.Stream(context.Background())
	rec := newRecorder()
	const path = "datadog/2/MANAGED_DEPLOYMENTS_DEBUG/ndm-1/config"

	p.Update(map[string]state.RawConfig{path: rawConfig(backendDocument)}, rec.callback)
	require.Len(t, drain(t, ch), 1)

	p.Update(map[string]state.RawConfig{}, rec.callback)

	changes := drain(t, ch)
	require.Len(t, changes, 1)
	assert.Len(t, changes[0].Unschedule, 2)
	assert.Empty(t, changes[0].Schedule)
}

func TestARepeatedIdenticalDocumentEmitsNothing(t *testing.T) {
	cfg := credentialsConfig(t)
	logComp := logmock.New(t)
	p, err := NewProvider(logComp, []handler.Handler{ndmsnmp.NewHandler(cfg, logComp)})
	require.NoError(t, err)

	ch := p.Stream(context.Background())
	rec := newRecorder()
	const path = "datadog/2/MANAGED_DEPLOYMENTS_DEBUG/ndm-1/config"

	p.Update(map[string]state.RawConfig{path: rawConfig(backendDocument)}, rec.callback)
	require.Len(t, drain(t, ch), 1, "the first delivery schedules the two devices")

	p.Update(map[string]state.RawConfig{path: rawConfig(backendDocument)}, rec.callback)

	assert.Empty(t, drain(t, ch), "an identical document must emit no schedule or unschedule churn")
	assert.Equal(t, state.ApplyStateAcknowledged, rec.states[path].State)
}

func TestADocumentWhoseInstancesAreAllUnresolvableSchedulesNothingAndErrors(t *testing.T) {
	cfg := credentialsConfig(t)
	logComp := logmock.New(t)
	p, err := NewProvider(logComp, []handler.Handler{ndmsnmp.NewHandler(cfg, logComp)})
	require.NoError(t, err)

	ch := p.Stream(context.Background())
	rec := newRecorder()
	const path = "datadog/2/MANAGED_DEPLOYMENTS_DEBUG/ndm-1/config"

	p.Update(map[string]state.RawConfig{path: rawConfig(`{"snmp":{"instances":[
		{"ip_address":"10.0.0.1","cred_id":"cred-unknown-1"},
		{"ip_address":"10.0.0.2","cred_id":"cred-unknown-2"}
	]}}`)}, rec.callback)

	changes := drain(t, ch)
	assert.Empty(t, changes, "nothing is scheduled when every instance is unresolvable")

	assert.Equal(t, state.ApplyStateError, rec.states[path].State)
	assert.Contains(t, rec.states[path].Error, "snmp: ")
	assert.Contains(t, rec.states[path].Error, "cred-unknown-1")
	assert.Contains(t, rec.states[path].Error, "cred-unknown-2")

	errs := p.GetConfigErrors()
	require.Contains(t, errs, path)
}

// credentialsConfig returns a config whose confd_path holds the credential
// file the delivered instances reference, at the path Fleet Automation writes.
func credentialsConfig(t *testing.T) model.BuildableConfig {
	t.Helper()

	confd := t.TempDir()
	path := filepath.Join(confd, "snmp.d", "snmp_credentials.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(`
credentials:
  - id: cred-abc
    snmp_version: "2c"
    community_string: public
`), 0o600))

	cfg := configmock.New(t)
	cfg.Set("confd_path", confd, model.SourceAgentRuntime)
	return cfg
}
