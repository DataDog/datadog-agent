// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preflightmode

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// atelMetric is one series of an agent telemetry metric.
type atelMetric struct {
	Value float64           `json:"value"`
	Tags  map[string]string `json:"tags"`
}

type atelMessage struct {
	RequestType string          `json:"request_type"`
	Payload     json.RawMessage `json:"payload"`
}

// parseAgentTelemetryMetrics returns the metrics in the output of
// `agent diagnose show-metadata agent-telemetry`, grouped by name. A metric with several tag sets
// is spread across several messages, one tag set per message.
//
// Copied from parseCOATMetrics in test/new-e2e/tests/gpu rather than shared, so that this suite
// does not have to depend on, or change, an unrelated one.
func parseAgentTelemetryMetrics(output string) (map[string][]atelMetric, error) {
	// The command output can contain log lines around the JSON payload.
	start := strings.Index(output, "{")
	end := strings.LastIndex(output, "}")
	if start < 0 || end < start {
		return nil, fmt.Errorf("no JSON payload in output: %s", output)
	}

	var top atelMessage
	if err := json.Unmarshal([]byte(output[start:end+1]), &top); err != nil {
		return nil, fmt.Errorf("failed to parse agent telemetry payload: %w", err)
	}

	var messages []atelMessage
	switch top.RequestType {
	case "agent-metrics":
		messages = []atelMessage{top}
	case "message-batch":
		if err := json.Unmarshal(top.Payload, &messages); err != nil {
			return nil, fmt.Errorf("failed to parse agent telemetry batch: %w", err)
		}
	default:
		return nil, fmt.Errorf("unexpected agent telemetry request type %q in output: %s", top.RequestType, output)
	}

	metrics := make(map[string][]atelMetric)
	for _, msg := range messages {
		if msg.RequestType != "agent-metrics" {
			continue
		}
		var payload struct {
			Metrics map[string]json.RawMessage `json:"metrics"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return nil, fmt.Errorf("failed to parse agent telemetry metrics: %w", err)
		}
		for name, raw := range payload.Metrics {
			if name == "agent_metadata" {
				continue
			}
			var m atelMetric
			if err := json.Unmarshal(raw, &m); err != nil {
				return nil, fmt.Errorf("failed to parse agent telemetry metric %s: %w", name, err)
			}
			metrics[name] = append(metrics[name], m)
		}
	}
	return metrics, nil
}

// tagKeys returns the keys of tags, so that a series' tag set can be compared without its values.
func tagKeys(tags map[string]string) []string {
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	return keys
}

// TestParseAgentTelemetryMetrics runs without any infrastructure. The fixtures are trimmed from
// what comp/core/agenttelemetry's getAsJSON produces for the data-plane-preflight-mode profile:
// a payload with nothing to report is an empty batch, and one with several metrics is a single
// agent-metrics message keyed by the profile's metric names. Empty label values are kept.
func TestParseAgentTelemetryMetrics(t *testing.T) {
	t.Run("empty batch", func(t *testing.T) {
		metrics, err := parseAgentTelemetryMetrics(`{"api_version":"v2","payload":[],"request_type":"message-batch"}`)
		require.NoError(t, err)
		assert.Empty(t, metrics)
	})

	t.Run("single message surrounded by log lines", func(t *testing.T) {
		output := `2026-11-24 17:31:48 UTC | CORE | INFO | some log line
{
	"api_version": "v2",
	"payload": {
		"message": "Agent metrics",
		"metrics": {
			"agent_metadata": {"hostname": "h", "os": "linux"},
			"data_plane.preflight_mode_duration_seconds": {"tags": {"emitter": "agent"}, "type": "gauge", "value": 181.5},
			"data_plane.preflight_mode_finding": {
				"tags": {"emitter": "agent", "finding": "spawn_failed", "source_file": "", "source_line": "", "spawn_error": "read_only", "spawn_stage": "workdir"},
				"type": "counter",
				"value": 1
			},
			"data_plane.preflight_mode_result": {"tags": {"emitter": "agent", "result": "spawn_failed"}, "type": "counter", "value": 1}
		}
	},
	"request_type": "agent-metrics"
}
`
		metrics, err := parseAgentTelemetryMetrics(output)
		require.NoError(t, err)
		assert.NotContains(t, metrics, "agent_metadata")

		require.Len(t, metrics[preflightModeFindingMetric], 1)
		finding := metrics[preflightModeFindingMetric][0]
		assert.ElementsMatch(t, preflightModeFindingTagKeys, tagKeys(finding.Tags))
		assert.Equal(t, "", finding.Tags["source_file"])
		assert.Equal(t, "workdir", finding.Tags["spawn_stage"])

		require.Len(t, metrics[preflightModeResultMetric], 1)
		assert.ElementsMatch(t, preflightModeResultTagKeys, tagKeys(metrics[preflightModeResultMetric][0].Tags))

		require.Len(t, metrics[preflightModeDurationMetric], 1)
		assert.InDelta(t, 181.5, metrics[preflightModeDurationMetric][0].Value, 0.001)
	})

	t.Run("batch with one tag set per message", func(t *testing.T) {
		output := `{"payload":[
			{"request_type":"agent-metrics","payload":{"metrics":{"data_plane.preflight_mode_finding":{"tags":{"finding":"errors_in_log"},"value":1}}}},
			{"request_type":"agent-metrics","payload":{"metrics":{"data_plane.preflight_mode_finding":{"tags":{"finding":"warnings_in_log"},"value":2}}}},
			{"request_type":"agent-logs","payload":{}}
		],"request_type":"message-batch"}`
		metrics, err := parseAgentTelemetryMetrics(output)
		require.NoError(t, err)
		assert.Len(t, metrics[preflightModeFindingMetric], 2)
	})

	t.Run("no JSON", func(t *testing.T) {
		_, err := parseAgentTelemetryMetrics("Error: agent-telemetry is not enabled")
		assert.Error(t, err)
	})
}
