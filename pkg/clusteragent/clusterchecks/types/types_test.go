// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNodeStatusCheckCompatibilityRoundTrip verifies the wire format of the
// check compatibility declaration between workers and the Cluster Agent:
// compat-bearing workers serialize the field, legacy workers (no compat) leave
// it absent so an older Cluster Agent reading the payload is unaffected and a
// new Cluster Agent reads them as unrestricted.
func TestNodeStatusCheckCompatibilityRoundTrip(t *testing.T) {
	// With compatibility
	status := NodeStatus{
		LastChange: 42,
		NodeType:   NodeTypeCLCRunner,
		CheckCompatibility: &CheckCompatibility{
			Include: []string{"kubernetes_state_core", "orchestrator"},
			Exclude: []string{"http_check"},
		},
	}
	data, err := json.Marshal(status)
	require.NoError(t, err)
	require.Contains(t, string(data), `"check_compatibility":{"include":["kubernetes_state_core","orchestrator"],"exclude":["http_check"]}`)

	var decoded NodeStatus
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.NotNil(t, decoded.CheckCompatibility)
	assert.Equal(t, []string{"kubernetes_state_core", "orchestrator"}, decoded.CheckCompatibility.Include)
	assert.Equal(t, []string{"http_check"}, decoded.CheckCompatibility.Exclude)
	assert.EqualValues(t, 42, decoded.LastChange)
	assert.Equal(t, NodeTypeCLCRunner, decoded.NodeType)

	// Without compatibility: the field must be absent from the payload so
	// legacy workers stay indistinguishable at the wire level.
	legacy := NodeStatus{LastChange: 7, NodeType: NodeTypeNodeAgent}
	data, err = json.Marshal(legacy)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "check_compatibility")

	var decodedLegacy NodeStatus
	require.NoError(t, json.Unmarshal(data, &decodedLegacy))
	assert.Nil(t, decodedLegacy.CheckCompatibility)

	// A payload missing the field entirely (older agent) decodes as
	// unrestricted.
	olderAgentPayload := []byte(`{"last_change":7,"node_type":2}`)
	var decodedOld NodeStatus
	require.NoError(t, json.Unmarshal(olderAgentPayload, &decodedOld))
	assert.Nil(t, decodedOld.CheckCompatibility)

	// Explicit empty lists decode as an unrestricted declaration (empty
	// include + empty exclude admits everything), not as a claim.
	emptyPayload := []byte(`{"last_change":9,"node_type":1,"check_compatibility":{"include":[],"exclude":[]}}`)
	var decodedEmpty NodeStatus
	require.NoError(t, json.Unmarshal(emptyPayload, &decodedEmpty))
	require.NotNil(t, decodedEmpty.CheckCompatibility)
	assert.Empty(t, decodedEmpty.CheckCompatibility.Include)
	assert.Empty(t, decodedEmpty.CheckCompatibility.Exclude)
}

func TestCheckCompatibilityAccepts(t *testing.T) {
	tests := []struct {
		name   string
		compat *CheckCompatibility
		check  string
		want   bool
	}{
		{"nil is unrestricted", nil, "any", true},
		{"empty is unrestricted", &CheckCompatibility{}, "any", true},
		{"included", &CheckCompatibility{Include: []string{"a"}}, "a", true},
		{"not included", &CheckCompatibility{Include: []string{"a"}}, "b", false},
		{"excluded", &CheckCompatibility{Exclude: []string{"a"}}, "a", false},
		{"not excluded", &CheckCompatibility{Exclude: []string{"a"}}, "b", true},
		{"exclude wins over include", &CheckCompatibility{Include: []string{"a"}, Exclude: []string{"a"}}, "a", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.compat.Accepts(tt.check))
		})
	}
}
