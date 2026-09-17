// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package status

import (
	"bytes"
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
		require.NoError(t, provider.TextBySection("apm agent", false, &output))
		assert.Equal(t, rawStatus, output.String())
	})
	t.Run("selected HTML", func(t *testing.T) {
		var output bytes.Buffer
		require.NoError(t, provider.HTMLBySection("apm agent", false, &output))
		assert.Contains(t, output.String(), "Trace Agent")
		assert.NotContains(t, output.String(), "Legacy Agent")
		assert.NotContains(t, output.String(), "No remote agents registered")
	})
	t.Run("selected unreachable agent", func(t *testing.T) {
		registry.statuses[0].FailureReason = "APM agent unreachable"
		provider := Provider{registry: registry}
		var output bytes.Buffer
		require.NoError(t, provider.TextBySection("APM Agent", false, &output))
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
		{RegisteredAgent: agents[0], JSONPayload: map[string]interface{}{"apmStats": "APM status"}},
		{RegisteredAgent: agents[1], JSONPayload: map[string]interface{}{"processAgentStatus": "Process status"}},
	}
	provider := Provider{registry: statusRegistry{agents: agents, statuses: statuses}}
	assert.Equal(t, []string{"APM Agent", "Process Agent"}, provider.Sections())

	stats := make(map[string]interface{})
	require.NoError(t, provider.JSON(false, stats))
	assert.Equal(t, agents, stats["registeredAgents"])
	assert.Equal(t, statuses, stats["registeredAgentStatuses"])
	assert.Equal(t, "APM status", stats["apmStats"])
	assert.Equal(t, "Process status", stats["processAgentStatus"])

	stats = make(map[string]interface{})
	require.NoError(t, provider.JSONBySection("apm agent", false, stats))
	assert.Equal(t, map[string]interface{}{"apmStats": "APM status"}, stats)
}

func TestJSONErrorsPreserveOtherPayloads(t *testing.T) {
	provider := Provider{registry: statusRegistry{
		statuses: []remoteagentregistry.StatusData{
			{JSONError: "invalid remote status JSON"},
			{JSONPayload: map[string]interface{}{"firstOnly": "first agent", "shared": "first value"}},
			{JSONPayload: map[string]interface{}{"secondOnly": "second agent", "shared": "second value", "stats": "remote value"}},
		},
	}}
	stats := map[string]interface{}{"stats": "local value"}
	err := provider.JSON(false, stats)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid remote status JSON")
	assert.Contains(t, err.Error(), `duplicate remote status JSON key "shared"`)
	assert.Contains(t, err.Error(), `duplicate remote status JSON key "stats"`)
	assert.Equal(t, "first value", stats["shared"])
	assert.Equal(t, "local value", stats["stats"])
	assert.Equal(t, "first agent", stats["firstOnly"])
	assert.Equal(t, "second agent", stats["secondOnly"])
}
