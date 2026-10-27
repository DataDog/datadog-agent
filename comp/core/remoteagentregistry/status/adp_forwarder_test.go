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
	views := expvarcompat.RootViews{RemoteByAgent: map[string]map[string]any{
		"agent-data-plane": {"Transactions": map[string]any{"Success": float64(2), "HTTPErrors": float64(3)}},
	}}
	stats := adpForwarderStatsFromViews(views)
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
	views.RemoteByAgent["agent-data-plane"] = map[string]any{"Transactions": map[string]any{"HTTPErrors": float64(3)}}
	info["adpForwarderStats"] = adpForwarderStatsFromViews(views)
	text.Reset()
	require.NoError(t, corestatus.RenderText(templatesFS, "adp_forwarder.tmpl", &text, info))
	assert.Contains(t, text.String(), "agent-data-plane:\n      HTTP errors: 3")
}

func TestCombinedForwarderStatusUsesOnlyMappedTotals(t *testing.T) {
	views := expvarcompat.RootViews{
		Core: map[string]any{"Transactions": map[string]any{"Success": float64(10)}},
		RemoteByAgent: map[string]map[string]any{
			"agent-data-plane": {"Transactions": map[string]any{
				"Success": float64(2), "ErrorsByType": map[string]any{"DNSErrors": float64(1)},
			}},
		},
		Combined: map[string]any{
			"Transactions": map[string]any{
				"Success": float64(12), "Errors": float64(4), "HTTPErrors": float64(3),
				"RetryQueueSize": float64(5), "ErrorsByType": map[string]any{
					"DNSErrors": float64(4), "TLSErrors": float64(5), "CoreOnlyErrors": float64(6),
				},
				"SuccessByEndpoint": map[string]any{"series": float64(8)},
			},
			"FileStorage": map[string]any{"FilesCount": float64(1)},
		},
	}
	stats := combinedForwarderStatsFromViews(views)
	transactions := stats["Transactions"].(map[string]any)
	assert.Equal(t, float64(12), transactions["Success"], "read the combined snapshot, not the Core or ADP view")
	assert.Equal(t, float64(4), transactions["Errors"])
	assert.Equal(t, float64(3), transactions["HTTPErrors"])
	assert.Equal(t, float64(5), transactions["RetryQueueSize"])
	assert.NotContains(t, transactions, "SuccessByEndpoint")
	assert.Equal(t, map[string]any{"DNSErrors": float64(4), "TLSErrors": float64(5)}, transactions["ErrorsByType"],
		"include a mapped Core error type even before ADP emits that series")
	assert.NotContains(t, stats, "FileStorage")

	provider := GetCombinedForwarderProvider(nil)
	assert.Equal(t, "Combined Forwarder", provider.Name())
	assert.Equal(t, "Combined Forwarder", provider.Section())
	info := map[string]interface{}{"combinedForwarderStats": stats}
	var text, html bytes.Buffer
	require.NoError(t, corestatus.RenderText(templatesFS, "combined_forwarder.tmpl", &text, info))
	require.NoError(t, corestatus.RenderHTML(templatesFS, "combined_forwarder_html.tmpl", &html, info))
	assert.Contains(t, text.String(), "Transactions (Core Agent + ADP)")
	assert.Contains(t, text.String(), "Success: 12")
	assert.Contains(t, text.String(), "DNSErrors: 4")
	assert.Contains(t, text.String(), "TLSErrors: 5")
	assert.NotContains(t, text.String(), "CoreOnlyErrors")
	assert.NotContains(t, text.String(), "SuccessByEndpoint")
	assert.Contains(t, html.String(), "Combined Forwarder Transactions (Core Agent + ADP)")
	assert.Contains(t, html.String(), "Success: 12")

	views.RemoteByAgent = nil
	assert.Empty(t, combinedForwarderStatsFromViews(views), "do not show Core-only totals as combined when ADP is absent")
	info["combinedForwarderStats"] = combinedForwarderStatsFromViews(views)
	text.Reset()
	require.NoError(t, corestatus.RenderText(templatesFS, "combined_forwarder.tmpl", &text, info))
	assert.Contains(t, text.String(), "No combined Forwarder telemetry available.")
}

func TestForwarderProvidersReadLiveExpvars(t *testing.T) {
	require.Nil(t, expvar.Get("forwarder"))
	require.Nil(t, expvar.Get("remote_agents"))

	core := expvar.NewMap("forwarder")
	transactions := new(expvar.Map).Init()
	coreSuccess := new(expvar.Int)
	coreSuccess.Set(10)
	transactions.Set("Success", coreSuccess)
	coreErrors := new(expvar.Map).Init()
	coreTLS := new(expvar.Int)
	coreTLS.Set(5)
	coreErrors.Set("TLSErrors", coreTLS)
	coreOnly := new(expvar.Int)
	coreOnly.Set(7)
	coreErrors.Set("CoreOnlyErrors", coreOnly)
	transactions.Set("ErrorsByType", coreErrors)
	core.Set("Transactions", transactions)

	source := expvarcompat.Source(func() map[string]any {
		return map[string]any{"agent-data-plane": map[string]any{"forwarder": map[string]any{
			"Transactions": map[string]any{
				"Success": int64(2), "ErrorsByType": map[string]any{"DNSErrors": int64(1)},
			},
		}}}
	})

	adp := make(map[string]interface{})
	require.NoError(t, GetADPForwarderProvider(source).JSON(false, adp))
	agents := adp["adpForwarderStats"].(map[string]map[string]any)
	assert.Equal(t, float64(2), agents["agent-data-plane"]["Transactions"].(map[string]any)["Success"])

	combined := make(map[string]interface{})
	provider := GetCombinedForwarderProvider(source)
	require.NoError(t, provider.JSON(false, combined))
	all := combined["combinedForwarderStats"].(map[string]any)["Transactions"].(map[string]any)
	assert.Equal(t, float64(12), all["Success"])
	assert.Equal(t, map[string]any{"DNSErrors": float64(1), "TLSErrors": float64(5)}, all["ErrorsByType"])
	assert.Equal(t, "10", transactions.Get("Success").String(), "status must not change Core expvars")
	assert.Nil(t, expvar.Get("remote_agents"), "ADP status reads the internal source, not a new expvar")

	var text, html bytes.Buffer
	require.NoError(t, provider.Text(false, &text))
	require.NoError(t, provider.HTML(false, &html))
	assert.Contains(t, text.String(), "Success: 12")
	assert.Contains(t, text.String(), "TLSErrors: 5")
	assert.NotContains(t, text.String(), "CoreOnlyErrors")
	assert.Contains(t, html.String(), "Success: 12")
}
