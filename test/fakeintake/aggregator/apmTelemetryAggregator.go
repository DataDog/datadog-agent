// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// AgentTelemetryLog represents a single log record from the agent's internal
// telemetry pipeline. It mirrors comp/core/agenttelemetry/impl/logs_payload.go::Log
// and is populated only for payloads whose request_type is "agent-logs".
type AgentTelemetryLog struct {
	Level         string `json:"level"`
	StackTrace    string `json:"stack_trace"`
	TracerTime    int64  `json:"tracer_time"`
	Count         int    `json:"count"`
	IsCrash       bool   `json:"is_crash"`
	Message       string `json:"message"`
	ErrorKind     string `json:"error_kind"`
	Tags          string `json:"tags"`
	collectedTime time.Time
}

// name returns the grouping key used by the aggregator. All agent-logs records
// share one key; per-hostname grouping is not needed for single-agent test VMs.
func (l *AgentTelemetryLog) name() string { return "agent-errortracking" }

// GetTags returns no tags — agent telemetry logs carry none on the wire.
func (l *AgentTelemetryLog) GetTags() []string { return []string{} }

// GetCollectedTime returns when the fakeintake server received this payload.
func (l *AgentTelemetryLog) GetCollectedTime() time.Time { return l.collectedTime }

// apmTelemetryEnvelope is the outer POST body sent to /api/v2/apmtelemetry.
// The same endpoint receives metrics, heartbeats, and other request types;
// request_type discriminates agent-logs records from the rest.
type apmTelemetryEnvelope struct {
	RequestType string `json:"request_type"`
	Payload     struct {
		Logs []*AgentTelemetryLog `json:"logs"`
	} `json:"payload"`
}

// ParseAgentTelemetryLogs parses one api.Payload into zero-or-more
// AgentTelemetryLog items. Payloads whose request_type is not "agent-logs"
// are silently skipped — the /api/v2/apmtelemetry endpoint is shared with
// agent metrics, heartbeats, and other telemetry types.
func ParseAgentTelemetryLogs(payload api.Payload) ([]*AgentTelemetryLog, error) {
	if bytes.Equal(payload.Data, []byte("{}")) {
		return []*AgentTelemetryLog{}, nil
	}

	inflated, err := inflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, err
	}

	// The /api/v2/apmtelemetry endpoint is shared: agent metrics, heartbeats,
	// and other payload types arrive here too, some encoded as protobuf. If
	// JSON unmarshal fails, skip silently — this payload is not agent-logs.
	var env apmTelemetryEnvelope
	if err := json.Unmarshal(inflated, &env); err != nil {
		return []*AgentTelemetryLog{}, nil
	}

	if env.RequestType != "agent-logs" {
		return []*AgentTelemetryLog{}, nil
	}

	for _, l := range env.Payload.Logs {
		l.collectedTime = payload.Timestamp
	}
	return env.Payload.Logs, nil
}

// AgentTelemetryLogAggregator aggregates AgentTelemetryLog payloads received
// on the /api/v2/apmtelemetry endpoint.
type AgentTelemetryLogAggregator struct {
	Aggregator[*AgentTelemetryLog]
}

// NewAgentTelemetryLogAggregator returns a new AgentTelemetryLogAggregator.
func NewAgentTelemetryLogAggregator() AgentTelemetryLogAggregator {
	return AgentTelemetryLogAggregator{
		Aggregator: newAggregator(ParseAgentTelemetryLogs),
	}
}
