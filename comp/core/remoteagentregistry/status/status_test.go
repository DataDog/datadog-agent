// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package status

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
)

type statusRegistry struct {
	remoteagentregistry.Component
	agents   []remoteagentregistry.RegisteredAgent
	statuses []remoteagentregistry.StatusData
}

func (r statusRegistry) GetRegisteredAgents() []remoteagentregistry.RegisteredAgent {
	return r.agents
}

func (r statusRegistry) GetRegisteredAgentStatuses() []remoteagentregistry.StatusData {
	return r.statuses
}

func TestStatusRendering(t *testing.T) {
	const rawStatus = "\nAPM <status> & details\n"
	agents := []remoteagentregistry.RegisteredAgent{
		{DisplayName: "Trace Agent", StatusSection: "APM Agent"},
		{DisplayName: "Legacy Agent"},
	}
	registry := statusRegistry{
		agents: agents,
		statuses: []remoteagentregistry.StatusData{
			{
				RegisteredAgent: agents[0],
				NamedSections: map[string]remoteagentregistry.StatusSection{
					"Details": {"": rawStatus},
				},
			},
			{RegisteredAgent: agents[1], FailureReason: "legacy agent unreachable"},
		},
	}
	provider := Provider{registry: registry}
	selected := provider.SectionProviders()[0]

	t.Run("full text", func(t *testing.T) {
		var output bytes.Buffer
		require.NoError(t, provider.Text(false, &output))
		assert.Contains(t, output.String(), "\nAPM Agent\n")
		assert.NotContains(t, output.String(), "\nTrace Agent\n")
		assert.Contains(t, output.String(), "\nLegacy Agent\n")
		assert.Contains(t, output.String(), rawStatus)
		assert.Contains(t, output.String(), "legacy agent unreachable")
	})
	t.Run("full HTML", func(t *testing.T) {
		var output bytes.Buffer
		require.NoError(t, provider.HTML(false, &output))
		assert.Regexp(t, `class="stat_title">\s*APM Agent\s*</span>`, output.String())
		assert.Regexp(t, `class="stat_title">\s*Legacy Agent\s*</span>`, output.String())
		assert.Contains(t, output.String(), "<pre><span>\nAPM &lt;status&gt; &amp; details\n</span></pre>")
	})
	t.Run("selected text", func(t *testing.T) {
		var output bytes.Buffer
		require.NoError(t, selected.Text(false, &output))
		assert.Equal(t, rawStatus, output.String())
	})
	t.Run("selected HTML", func(t *testing.T) {
		var output bytes.Buffer
		require.NoError(t, selected.HTML(false, &output))
		assert.Contains(t, output.String(), "Trace Agent")
		assert.NotContains(t, output.String(), "Legacy Agent")
		assert.NotContains(t, output.String(), "No remote agents registered")
	})
	t.Run("selected unreachable agent", func(t *testing.T) {
		registry.statuses[0].FailureReason = "APM agent unreachable"
		var output bytes.Buffer
		require.NoError(t, selected.Text(false, &output))
		assert.Equal(t, "APM agent unreachable", output.String())
	})
}

func TestStatusJSONAndSections(t *testing.T) {
	agents := []remoteagentregistry.RegisteredAgent{
		{DisplayName: "Trace Agent", StatusSection: "APM Agent"},
		{DisplayName: "Process Agent", StatusSection: "Process Agent"},
		{StatusSection: "apm agent"}, // Duplicate section names are case-insensitive.
		{},                           // Older agents do not advertise sections.
	}
	statuses := []remoteagentregistry.StatusData{
		{RegisteredAgent: agents[0], JSONPayload: map[string]json.RawMessage{"apmStats": json.RawMessage(`{"receiver":"running"}`)}},
		{RegisteredAgent: agents[1], JSONPayload: map[string]json.RawMessage{"processAgentStatus": json.RawMessage(`{"pid":123}`)}},
	}
	provider := Provider{registry: statusRegistry{agents: agents, statuses: statuses}}
	sections := provider.SectionProviders()
	require.Len(t, sections, 2)
	assert.Equal(t, "APM Agent", sections[0].Section())
	assert.Equal(t, "Process Agent", sections[1].Section())

	stats := make(map[string]interface{})
	require.NoError(t, provider.JSON(false, stats))
	assert.Equal(t, agents, stats["registeredAgents"])
	assert.Equal(t, statuses, stats["registeredAgentStatuses"])
	assert.Equal(t, json.RawMessage(`{"receiver":"running"}`), stats["apmStats"])
	assert.Equal(t, json.RawMessage(`{"pid":123}`), stats["processAgentStatus"])

	stats = make(map[string]interface{})
	require.NoError(t, sections[0].JSON(false, stats))
	encoded, err := json.Marshal(stats)
	require.NoError(t, err)
	assert.JSONEq(t, `{"apmStats":{"receiver":"running"}}`, string(encoded))
}

func TestJSONErrorsPreserveOtherPayloads(t *testing.T) {
	provider := Provider{registry: statusRegistry{
		statuses: []remoteagentregistry.StatusData{
			{JSONError: "invalid remote status JSON"},
			{JSONPayload: map[string]json.RawMessage{"firstOnly": json.RawMessage(`"first agent"`), "shared": json.RawMessage(`"first value"`)}},
			{JSONPayload: map[string]json.RawMessage{"secondOnly": json.RawMessage(`"second agent"`), "shared": json.RawMessage(`"second value"`), "stats": json.RawMessage(`"remote value"`), "errors": json.RawMessage(`[]`)}},
		},
	}}
	stats := map[string]interface{}{"stats": "local value", "registeredAgents": "local value"}
	err := provider.JSON(false, stats)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid remote status JSON")
	assert.Contains(t, err.Error(), `duplicate remote status JSON key "shared"`)
	assert.Contains(t, err.Error(), `duplicate remote status JSON key "stats"`)
	assert.Contains(t, err.Error(), `duplicate remote status JSON key "registeredAgents"`)
	assert.Contains(t, err.Error(), `reserved remote status JSON key "errors"`)
	assert.NotContains(t, stats, "errors")
	assert.Equal(t, "local value", stats["registeredAgents"])
	assert.Equal(t, json.RawMessage(`"first value"`), stats["shared"])
	assert.Equal(t, "local value", stats["stats"])
	assert.Equal(t, json.RawMessage(`"first agent"`), stats["firstOnly"])
	assert.Equal(t, json.RawMessage(`"second agent"`), stats["secondOnly"])
}

func TestUnavailableStatusJSON(t *testing.T) {
	provider := Provider{section: "Process Agent", registry: statusRegistry{statuses: []remoteagentregistry.StatusData{{
		RegisteredAgent: remoteagentregistry.RegisteredAgent{DisplayName: "Process Agent", StatusSection: "Process Agent"},
		FailureReason:   "connection refused",
	}}}}
	err := provider.JSON(false, make(map[string]interface{}))
	require.ErrorContains(t, err, "connection refused")
}
