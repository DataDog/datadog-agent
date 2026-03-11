// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package admission

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admiv1 "k8s.io/api/admission/v1"

	admicommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
)

func TestIsProbe(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		expected bool
	}{
		{
			name:     "object with probe label",
			raw:      `{"metadata":{"labels":{"` + admicommon.ProbeLabelKey + `":"true"}}}`,
			expected: true,
		},
		{
			name:     "object without probe label",
			raw:      `{"metadata":{"labels":{"app":"nginx"}}}`,
			expected: false,
		},
		{
			name:     "object with no labels",
			raw:      `{"metadata":{}}`,
			expected: false,
		},
		{
			name:     "invalid JSON",
			raw:      `not json`,
			expected: false,
		},
		{
			name:     "empty object",
			raw:      `{}`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isProbe([]byte(tt.raw)))
		})
	}
}

func TestProbeResponse_NonProbeObject(t *testing.T) {
	raw := []byte(`{"metadata":{"labels":{"app":"nginx"}}}`)
	resp := probeResponse(raw)
	assert.Nil(t, resp)
}

func TestProbeResponse_ProbeObject(t *testing.T) {
	raw := []byte(`{"metadata":{"labels":{"` + admicommon.ProbeLabelKey + `":"true"}}}`)
	resp := probeResponse(raw)
	require.NotNil(t, resp)

	assert.True(t, resp.Allowed)
	assert.NotNil(t, resp.PatchType)
	assert.Equal(t, admiv1.PatchTypeJSONPatch, *resp.PatchType)

	var patch []map[string]interface{}
	err := json.Unmarshal(resp.Patch, &patch)
	require.NoError(t, err)
	require.Len(t, patch, 1)
	assert.Equal(t, "add", patch[0]["op"])
	assert.Equal(t, "/metadata/annotations", patch[0]["path"])

	annotations, ok := patch[0]["value"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "true", annotations[admicommon.ProbeReceivedAnnotationKey])
}
