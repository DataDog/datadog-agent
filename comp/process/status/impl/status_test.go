// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"expvar"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

//go:embed fixtures/expvar_response.tmpl
var fixturesTemplates embed.FS

func TestStatusFromLocalExpvars(t *testing.T) {
	fixture, err := fixturesTemplates.ReadFile("fixtures/expvar_response.tmpl")
	require.NoError(t, err)
	var values map[string]map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(fixture, &values))
	local, _ := expvar.Get("process_agent").(*expvar.Map)
	if local == nil {
		local = expvar.NewMap("process_agent")
	}
	for key, value := range values["process_agent"] {
		original := local.Get(key)
		local.Set(key, expvar.Func(func() interface{} { return value }))
		t.Cleanup(func() {
			if original == nil {
				local.Delete(key)
			} else {
				local.Set(key, original)
			}
		})
	}

	cfg := config.NewMock(t)
	cfg.SetInTest("cloud_provider_metadata", []string{})
	cfg.SetInTest("process_config.expvar_port", 0) // No HTTP status server.
	provider := statusProvider{config: cfg, hostname: hostnameimpl.NewHostnameService()}

	response, err := provider.GetStatusDetails(context.Background(), &pbcore.GetStatusDetailsRequest{})
	require.NoError(t, err)
	var payload map[string]map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(response.JsonPayload, &payload))
	require.NotContains(t, payload["processAgentStatus"], "error")
	assert.JSONEq(t, string(fixture), string(payload["processAgentStatus"]["expvars"]))
	require.Contains(t, response.NamedSections, "Details")
	assert.Contains(t, response.NamedSections["Details"].Fields[""], "Pid: 72211")
	assert.Contains(t, response.NamedSections["Details"].Fields[""], "Enabled Checks: [process rtprocess]")

	stats := make(map[string]interface{})
	require.NoError(t, provider.JSON(false, stats))
	encoded, err := json.Marshal(stats)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(encoded, &payload))
	assert.JSONEq(t, string(fixture), string(payload["processAgentStatus"]["expvars"]))
	var output bytes.Buffer
	require.NoError(t, provider.Text(false, &output))
	assert.Contains(t, output.String(), "API Key ending with:")

	local.Set("pid", expvar.Func(func() interface{} { return "invalid pid" }))
	output.Reset()
	require.NoError(t, provider.Text(false, &output))
	assert.Contains(t, output.String(), "Status: Not running or unreachable")
}
