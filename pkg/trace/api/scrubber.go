// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package api

import (
	"encoding/json"
	"net/http"
	"regexp"
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

// sensitiveArgFlags are flag names whose value must be redacted even when
// passed as a separate argv token from the flag (e.g. "--password hunter2")
// rather than joined with "=" or ":", which pkg/util/scrubber's default
// replacers already handle. Mirrors the word list process-agent's cmdline
// scrubber (pkg/process/procutil.DataScrubber) uses; that package can't be
// imported here because pkg/trace is a standalone Go module and procutil
// lives in the (much larger) root module.
var sensitiveArgFlags = []string{
	"password", "passwd", "pwd", "mysql_pwd",
	"access_token", "auth_token", "token",
	"api_key", "apikey", "secret", "credentials",
}

// cmdLineScrubber adds the space-delimited flag/value replacers on top of
// pkg/util/scrubber's default patterns.
var cmdLineScrubber = newCmdLineScrubber()

func newCmdLineScrubber() *scrubber.Scrubber {
	s := scrubber.NewWithDefaults()
	for _, word := range sensitiveArgFlags {
		re := regexp.MustCompile(`(?i)((?:-{1,2})?` + word + `)( +)([^\s]+)`)
		s.AddReplacer(scrubber.SingleLine, scrubber.Replacer{
			Regex: re,
			Repl:  []byte(`$1$2********`),
		})
	}
	return s
}

// scrubCommandLine redacts secrets from a raw command line string, covering
// both "--password=hunter2"/"password: hunter2" forms (pkg/util/scrubber's
// defaults) and the space-delimited "--password hunter2" form (added by
// cmdLineScrubber above). It cannot redact a secret passed as a bare
// positional argument with no recognizable flag name (e.g. "mysql root
// hunter2").
func scrubCommandLine(cmdLine string) string {
	return cmdLineScrubber.ScrubLine(cmdLine)
}

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

	scrubbed := scrubCommandLine(payload.CommandLine)
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
