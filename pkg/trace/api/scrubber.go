// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// telemetryRequestTypeHeader names the header tracer libraries use to declare
// the kind of telemetry payload they are sending.
const telemetryRequestTypeHeader = "DD-Telemetry-Request-Type"

// apmTelemetryRequestType is the value of telemetryRequestTypeHeader used by
// the SSI telemetry forwarder to report a single injection attempt.
const apmTelemetryRequestType = "injection-metadata"

// apmTelemetryProxyPath is the request path (after /telemetry/proxy is
// stripped by the route mux) that carries APM library telemetry.
const apmTelemetryProxyPath = "/api/v2/apmtelemetry"

// scrubberLogger throttles repeated decode failures so a misbehaving tracer
// can't drown the agent log.
var scrubberLogger = log.NewThrottled(5, 10*time.Second)

// telemetryRequest is a partial decode of the APM library telemetry envelope
// (see https://github.com/DataDog/instrumentation-telemetry-api-docs). Only
// the field carrying the typed payload needs to be addressable; the rest are
// kept as raw JSON so re-marshalling does not drop or reorder tracer-supplied
// fields the agent does not care about.
type telemetryRequest struct {
	APIVersion  string          `json:"api_version"`
	RequestType string          `json:"request_type"`
	TracerTime  int64           `json:"tracer_time"`
	RuntimeID   string          `json:"runtime_id"`
	SeqID       int64           `json:"seq_id"`
	Application json.RawMessage `json:"application"`
	Host        json.RawMessage `json:"host"`
	Payload     json.RawMessage `json:"payload"`
	Debug       bool            `json:"debug,omitempty"`
}

// injectionMetadata is the payload sent inside a telemetryRequest with
// request_type=injection-metadata by the SSI tracer-injection sidecar.
type injectionMetadata struct {
	Component        string `json:"component"`
	ComponentVersion string `json:"component_version"`
	Result           string `json:"result"`
	ResultReason     string `json:"result_reason"`
	ResultClass      string `json:"result_class"`
	RuntimeID        string `json:"runtime_id"`
	CommandLine      string `json:"command_line"`
	TimestampMillis  int64  `json:"timestamp_millis"`
	CreateTimeMillis int64  `json:"create_time_millis"`
	Language         string `json:"language"`
}

// stripCommandLineSecrets returns body with the command_line field of an
// injection-metadata payload redacted by the default scrubber. It is a no-op
// for any request that is not an APM injection-metadata payload, and for
// payloads that fail to decode — the latter so a malformed body is still
// proxied to the intake (where it can be observed) rather than silently
// dropped here.
func stripCommandLineSecrets(req *http.Request, body []byte) []byte {
	if req.Header.Get(telemetryRequestTypeHeader) != apmTelemetryRequestType {
		return body
	}
	if req.URL.Path != apmTelemetryProxyPath {
		return body
	}

	var msg telemetryRequest
	if err := json.Unmarshal(body, &msg); err != nil {
		scrubberLogger.Error("telemetry proxy: failed to decode injection-metadata envelope: %v", err)
		return body
	}
	if len(msg.Payload) == 0 {
		return body
	}

	var payload injectionMetadata
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		scrubberLogger.Error("telemetry proxy: failed to decode injection-metadata payload: %v", err)
		return body
	}
	if payload.CommandLine == "" {
		return body
	}

	scrubbed := scrubber.ScrubLine(payload.CommandLine)
	if scrubbed == payload.CommandLine {
		return body
	}
	payload.CommandLine = scrubbed

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		scrubberLogger.Error("telemetry proxy: failed to re-encode injection-metadata payload: %v", err)
		return body
	}
	msg.Payload = rawPayload

	out, err := json.Marshal(msg)
	if err != nil {
		scrubberLogger.Error("telemetry proxy: failed to re-encode injection-metadata envelope: %v", err)
		return body
	}
	return out
}
