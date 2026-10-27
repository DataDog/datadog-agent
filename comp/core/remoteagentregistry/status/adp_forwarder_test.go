// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package status

import (
	"bytes"
	"expvar"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/expvarcompat"
	corestatus "github.com/DataDog/datadog-agent/comp/core/status"
)

func TestADPForwarderStatus(t *testing.T) {
	snapshot := map[string]any{
		"agent-data-plane": map[string]any{"forwarder": map[string]any{
			"Transactions": map[string]any{"Success": int64(2), "HTTPErrors": int64(3)},
		}},
		"process-agent": map[string]any{"process": map[string]any{"CPU": int64(1)}},
	}
	stats := adpForwarderStatsFromSnapshot(snapshot)
	assert.Contains(t, stats, "agent-data-plane")
	assert.NotContains(t, stats, "process-agent")

	provider := GetADPForwarderProvider(nil)
	assert.Equal(t, "ADP Forwarder", provider.Name())
	assert.Equal(t, "ADP Forwarder", provider.Section())

	info := map[string]interface{}{"adpForwarderStats": stats}
	var text, html bytes.Buffer
	require.NoError(t, corestatus.RenderText(templatesFS, "adp_forwarder.tmpl", &text, info))
	require.NoError(t, corestatus.RenderHTML(templatesFS, "adp_forwarder_html.tmpl", &html, info))
	assert.Contains(t, text.String(), "agent-data-plane:\n      Success: 2\n      HTTP errors: 3")
	assert.Contains(t, html.String(), "agent-data-plane")
	assert.Contains(t, html.String(), "Success: 2")

	// A lone HTTP error must not run into the agent label.
	snapshot["agent-data-plane"] = map[string]any{"forwarder": map[string]any{
		"Transactions": map[string]any{"HTTPErrors": int64(3)},
	}}
	info["adpForwarderStats"] = adpForwarderStatsFromSnapshot(snapshot)
	text.Reset()
	require.NoError(t, corestatus.RenderText(templatesFS, "adp_forwarder.tmpl", &text, info))
	assert.Contains(t, text.String(), "agent-data-plane:\n      HTTP errors: 3")
}

func TestADPForwarderProviderReadsInternalSnapshot(t *testing.T) {
	require.Nil(t, expvar.Get("forwarder"))

	core := expvar.NewMap("forwarder")
	transactions := new(expvar.Map).Init()
	coreSuccess := new(expvar.Int)
	coreSuccess.Set(10)
	transactions.Set("Success", coreSuccess)
	coreErrors := new(expvar.Map).Init()
	coreTLS := new(expvar.Int)
	coreTLS.Set(5)
	coreErrors.Set("TLSErrors", coreTLS)
	transactions.Set("ErrorsByType", coreErrors)
	core.Set("Transactions", transactions)

	source := expvarcompat.Source(func() map[string]any {
		return map[string]any{"agent-data-plane": map[string]any{"forwarder": map[string]any{
			"Transactions": map[string]any{
				"Success": int64(2), "ErrorsByType": map[string]any{"DNSErrors": int64(1)},
			},
		}}}
	})

	provider := GetADPForwarderProvider(source)
	stats := make(map[string]interface{})
	require.NoError(t, provider.JSON(false, stats))
	agents := stats["adpForwarderStats"].(map[string]map[string]any)
	assert.Equal(t, int64(2), agents["agent-data-plane"]["Transactions"].(map[string]any)["Success"])
	assert.Equal(t, map[string]any{"DNSErrors": int64(1)}, agents["agent-data-plane"]["Transactions"].(map[string]any)["ErrorsByType"])
	assert.Equal(t, "10", transactions.Get("Success").String(), "Core expvars remain Core-only")

	var text, html bytes.Buffer
	require.NoError(t, provider.Text(false, &text))
	require.NoError(t, provider.HTML(false, &html))
	assert.Contains(t, text.String(), "Success: 2")
	assert.Contains(t, text.String(), "DNSErrors: 1")
	assert.NotContains(t, text.String(), "TLSErrors")
	assert.Contains(t, html.String(), "Success: 2")

	empty := make(map[string]interface{})
	require.NoError(t, GetADPForwarderProvider(nil).JSON(false, empty))
	assert.Empty(t, empty["adpForwarderStats"])
}
