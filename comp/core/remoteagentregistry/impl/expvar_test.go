// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteagentregistryimpl

import (
	"context"
	"encoding/json"
	"expvar"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const adpTransactionsPromText = `
# TYPE transactions__success counter
transactions__success{domain="https://api.datadoghq.com",endpoint="series_v2",proto_version="HTTP/2.0"} 40
transactions__success{domain="https://api.datadoghq.com",endpoint="sketches_v2",proto_version="HTTP/2.0"} 2
# TYPE transactions__success_bytes counter
transactions__success_bytes{domain="https://api.datadoghq.com",endpoint="series_v2"} 1024
`

// localTransactionsSuccess stands in for the forwarder's `forwarder/Transactions/Success` expvar, which isn't linked
// into this test binary.
var localTransactionsSuccess = func() *expvar.Int {
	transactions := new(expvar.Map).Init()
	success := new(expvar.Int)
	transactions.Set("Success", success)
	expvar.NewMap("forwarder").Set("Transactions", transactions)
	return success
}()

func getExpvarJSON(t *testing.T, name string) map[string]any {
	v := expvar.Get(name)
	require.NotNil(t, v, name)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(v.String()), &got))
	return got
}

func TestRemoteAgentExpvarsPerAgent(t *testing.T) {
	provides, lc, _, _, ipcComp := buildComponent(t)
	lc.Start(context.Background())

	_ = buildAndRegisterRemoteAgent(t, ipcComp, provides.Comp, "agent-data-plane", "Agent Data Plane", "123",
		withTelemetryProvider(adpTransactionsPromText),
	)

	assert.Equal(t, map[string]any{
		"agent-data-plane": map[string]any{
			"forwarder": map[string]any{
				"Transactions": map[string]any{
					"Success": float64(42),
				},
			},
		},
	}, getExpvarJSON(t, remoteAgentsExpvarName))
}

func TestRemoteAgentExpvarsAddedToCoreAgentExpvars(t *testing.T) {
	localTransactionsSuccess.Set(8)

	provides, lc, _, _, ipcComp := buildComponent(t)
	lc.Start(context.Background())

	transactions := getExpvarJSON(t, "forwarder")["Transactions"].(map[string]any)
	assert.Equal(t, float64(8), transactions["Success"], "only the local value before any remote agent registers")

	_ = buildAndRegisterRemoteAgent(t, ipcComp, provides.Comp, "agent-data-plane", "Agent Data Plane", "123",
		withTelemetryProvider(adpTransactionsPromText),
	)
	provides.Comp.(*remoteAgentRegistry).expvarCache.values = nil

	transactions = getExpvarJSON(t, "forwarder")["Transactions"].(map[string]any)
	assert.Equal(t, float64(50), transactions["Success"], "local value plus the remote agent's value")

	// The local value keeps updating through the wrapper.
	localTransactionsSuccess.Add(1)
	transactions = getExpvarJSON(t, "forwarder")["Transactions"].(map[string]any)
	assert.Equal(t, float64(51), transactions["Success"])
}

func TestExpvarValuesFromPromTextSkipsUnmappedMetrics(t *testing.T) {
	assert.Empty(t, expvarValuesFromPromText(`
# TYPE some_other_metric counter
some_other_metric 1
`, "agent-data-plane"))
}
