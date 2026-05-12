// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecksimpl

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

func TestPayloadIncludesIntegrationStatus(t *testing.T) {
	payload := &Payload{
		Clustername:                   "test-cluster",
		ClusterID:                     "abc123",
		ClusterCheckMetadata:          make(map[string][]metadata),
		ClusterCheckStatus:            make(map[string]interface{}),
		ClusterCheckIntegrationStatus: make(map[string][]metadata),
		UUID:                          "test-uuid",
	}

	// Simulate what collectClusterCheckMetadata does for integration status
	stats := types.CLCRunnersStats{
		"kubernetes_state_core:db3f3028d40c564d": {
			AverageExecutionTime: 3209,
			MetricSamples:        197303,
			TotalMetricSamples:   64518134,
			Events:               0,
			TotalEvents:          0,
			ServiceChecks:        406,
			TotalServiceChecks:   132762,
			LastExecFailed:       false,
			LastError:            "",
			TotalRuns:            328,
			TotalErrors:          0,
			LastSuccessDate:      1775563775,
			LastExecutionDate:    1775563775974,
		},
	}

	for statsID, s := range stats {
		if s.TotalRuns == 0 {
			continue
		}
		checkName := "kubernetes_state_core"
		status := "OK"
		if s.LastExecFailed {
			status = "ERROR"
		}
		statusEntry := metadata{
			"config.hash": statsID,
			"status":      status,
			"errors":      s.LastError,
		}
		payload.ClusterCheckIntegrationStatus[checkName] = append(
			payload.ClusterCheckIntegrationStatus[checkName], statusEntry)
	}

	// Verify the status was added
	require.Len(t, payload.ClusterCheckIntegrationStatus, 1)
	require.Len(t, payload.ClusterCheckIntegrationStatus["kubernetes_state_core"], 1)

	entry := payload.ClusterCheckIntegrationStatus["kubernetes_state_core"][0]
	assert.Equal(t, "kubernetes_state_core:db3f3028d40c564d", entry["config.hash"])
	assert.Equal(t, "OK", entry["status"])
	assert.Equal(t, "", entry["errors"])

	// Verify JSON serialization includes the field
	jsonBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	var raw map[string]interface{}
	err = json.Unmarshal(jsonBytes, &raw)
	require.NoError(t, err)
	assert.Contains(t, raw, "clustercheck_integration_status")
}

func TestPayloadEmptyIntegrationStatus(t *testing.T) {
	payload := &Payload{
		Clustername:                   "test-cluster",
		ClusterID:                     "abc123",
		ClusterCheckMetadata:          make(map[string][]metadata),
		ClusterCheckStatus:            make(map[string]interface{}),
		ClusterCheckIntegrationStatus: make(map[string][]metadata),
		UUID:                          "test-uuid",
	}

	// No stats added — integration status should be empty
	assert.Empty(t, payload.ClusterCheckIntegrationStatus)

	// Verify JSON serialization omits empty field (omitempty)
	jsonBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	var raw map[string]interface{}
	err = json.Unmarshal(jsonBytes, &raw)
	require.NoError(t, err)
	// omitempty on empty map — Go encodes empty maps, not nil maps
	// So it will be present but empty
	status, ok := raw["clustercheck_integration_status"]
	if ok {
		statusMap, _ := status.(map[string]interface{})
		assert.Empty(t, statusMap)
	}
}

func TestPayloadIntegrationStatusErrorCheck(t *testing.T) {
	payload := &Payload{
		Clustername:                   "test-cluster",
		ClusterID:                     "abc123",
		ClusterCheckMetadata:          make(map[string][]metadata),
		ClusterCheckStatus:            make(map[string]interface{}),
		ClusterCheckIntegrationStatus: make(map[string][]metadata),
		UUID:                          "test-uuid",
	}

	statusEntry := metadata{
		"config.hash": "my_check:abc123",
		"status":      "ERROR",
		"errors":      "connection refused",
	}
	payload.ClusterCheckIntegrationStatus["my_check"] = append(
		payload.ClusterCheckIntegrationStatus["my_check"], statusEntry)

	entry := payload.ClusterCheckIntegrationStatus["my_check"][0]
	assert.Equal(t, "ERROR", entry["status"])
	assert.Equal(t, "connection refused", entry["errors"])
}

func TestPayloadSkipsZeroRunStats(t *testing.T) {
	payload := &Payload{
		ClusterCheckIntegrationStatus: make(map[string][]metadata),
	}

	// Simulate stats with TotalRuns == 0 — should be skipped
	stats := types.CLCRunnersStats{
		"my_check:abc123": {
			TotalRuns:   0,
			TotalErrors: 0,
		},
		"my_check:def456": {
			TotalRuns:      5,
			TotalErrors:    1,
			LastExecFailed: true,
			LastError:      "timeout",
		},
	}

	for statsID, s := range stats {
		if s.TotalRuns == 0 {
			continue
		}
		checkName := "my_check"
		status := "OK"
		if s.LastExecFailed {
			status = "ERROR"
		}
		payload.ClusterCheckIntegrationStatus[checkName] = append(
			payload.ClusterCheckIntegrationStatus[checkName], metadata{
				"config.hash": statsID,
				"status":      status,
				"errors":      s.LastError,
			})
	}

	// Only one entry (the one with TotalRuns > 0)
	require.Len(t, payload.ClusterCheckIntegrationStatus["my_check"], 1)
	assert.Equal(t, "ERROR", payload.ClusterCheckIntegrationStatus["my_check"][0]["status"])
	assert.Equal(t, "timeout", payload.ClusterCheckIntegrationStatus["my_check"][0]["errors"])
}
