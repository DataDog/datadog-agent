// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// newTestForwarder returns a TelemetryForwarder with just enough state
// (cmdLineScrubber, logger) to exercise stripCommandLineSecrets in isolation.
func newTestForwarder(t *testing.T) *TelemetryForwarder {
	t.Helper()
	return newTestReceiverFromConfig(newTestReceiverConfig()).telemetryForwarder
}

// makeInjectionMetadataBody returns a complete telemetryRequest envelope
// wrapping an injectionMetadata payload with the given command line.
func makeInjectionMetadataBody(t *testing.T, cmdLine string) []byte {
	t.Helper()
	return makeInjectionMetadataBodyWithMetadata(t, cmdLine, nil)
}

// makeInjectionMetadataBodyWithMetadata is like makeInjectionMetadataBody but
// also sets the free-form metadata field, to exercise metadata scrubbing.
func makeInjectionMetadataBodyWithMetadata(t *testing.T, cmdLine string, metadata json.RawMessage) []byte {
	t.Helper()
	payloadBytes, err := json.Marshal(injectionMetadata{
		Component:        "python",
		ComponentVersion: "3.5.1",
		Result:           "injected",
		ResultReason:     "injection succeeded",
		ResultClass:      "success",
		RuntimeID:        "f81d4fae-7dec-11d0-a765-00a0c91e6bf6",
		CommandLine:      cmdLine,
		TimestampMillis:  1746722642,
		CreateTimeMillis: 1746722640,
		Language:         "python",
		Metadata:         metadata,
	})
	assert.NoError(t, err)
	envBytes, err := json.Marshal(telemetryRequest{
		APIVersion:  "v2",
		RequestType: apmTelemetryRequestType,
		TracerTime:  1746722643,
		RuntimeID:   "f81d4fae-7dec-11d0-a765-00a0c91e6bf6",
		SeqID:       1,
		Application: json.RawMessage(`{"service_name":"test_service","language_name":"python","tracer_version":"1.0.0"}`),
		Host:        json.RawMessage(`{"hostname":"test_host","container_id":"test_cid"}`),
		Payload:     payloadBytes,
	})
	assert.NoError(t, err)
	return envBytes
}

// newInjectionMetadataReq builds an HTTP request modeling what a tracer
// library would send to the proxy (path post-StripPrefix, correct headers).
// Mutators can override per-test (e.g. to drop the header or change the path).
func newInjectionMetadataReq(t *testing.T, cmdLine string, opts ...func(*http.Request)) (*http.Request, []byte) {
	t.Helper()
	body := makeInjectionMetadataBody(t, cmdLine)
	req, err := http.NewRequest("POST", apmTelemetryProxyPath, bytes.NewReader(body))
	assert.NoError(t, err)
	req.Header.Set(telemetryRequestTypeHeader, apmTelemetryRequestType)
	for _, opt := range opts {
		opt(req)
	}
	return req, body
}

func decodeCommandLine(t *testing.T, body []byte) string {
	t.Helper()
	var env telemetryRequest
	assert.NoError(t, json.Unmarshal(body, &env))
	var p injectionMetadata
	assert.NoError(t, json.Unmarshal(env.Payload, &p))
	return p.CommandLine
}

func decodeMetadata(t *testing.T, body []byte) json.RawMessage {
	t.Helper()
	var env telemetryRequest
	assert.NoError(t, json.Unmarshal(body, &env))
	var p injectionMetadata
	assert.NoError(t, json.Unmarshal(env.Payload, &p))
	return p.Metadata
}

func TestStripCommandLineSecrets_DoesNotApply(t *testing.T) {
	cases := []struct {
		name string
		opt  func(*http.Request)
	}{
		{
			name: "header missing",
			opt:  func(r *http.Request) { r.Header.Del(telemetryRequestTypeHeader) },
		},
		{
			name: "header has wrong value",
			opt:  func(r *http.Request) { r.Header.Set(telemetryRequestTypeHeader, "app-started") },
		},
		{
			name: "path is not apmtelemetry",
			opt:  func(r *http.Request) { r.URL.Path = "/api/v2/something-else" },
		},
		{
			name: "path is empty",
			opt:  func(r *http.Request) { r.URL.Path = "" },
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, body := newInjectionMetadataReq(t, "/usr/bin/python --password=hunter2 app.py", c.opt)
			out := newTestForwarder(t).stripCommandLineSecrets(req, body)
			assert.Equal(t, body, out, "body must be returned untouched when the gate does not match")
			assert.Contains(t, decodeCommandLine(t, out), "hunter2", "untouched body still contains the secret — that is the point of this test")
		})
	}
}

func TestStripCommandLineSecrets_RedactsSecret(t *testing.T) {
	cases := []struct {
		name        string
		cmdLine     string
		mustOmit    []string
		mustContain []string
	}{
		{
			name:        "long-form password flag",
			cmdLine:     "/usr/bin/python app.py --password=hunter2",
			mustOmit:    []string{"hunter2"},
			mustContain: []string{"********"},
		},
		{
			name:        "uppercase PASSWORD flag",
			cmdLine:     "/usr/bin/python app.py --PASSWORD=hunter2",
			mustOmit:    []string{"hunter2"},
			mustContain: []string{"********"},
		},
		{
			name:        "pwd flag",
			cmdLine:     "/usr/bin/mysql --pwd=letmein",
			mustOmit:    []string{"letmein"},
			mustContain: []string{"********"},
		},
		{
			name:     "api_key flag with 32-hex token",
			cmdLine:  "/usr/bin/agent --api_key=abcdef0123456789abcdef0123456789abcd",
			mustOmit: []string{"abcdef0123456789abcdef0123456789"},
		},
		{
			name:     "bearer token in argv",
			cmdLine:  "/usr/bin/curl -H Authorization: Bearer abcdefghij0123456789",
			mustOmit: []string{"abcdefghij0123456789"},
		},
		{
			name:        "uri credentials",
			cmdLine:     "/usr/bin/psql postgres://user:supersecret@db:5432/x",
			mustOmit:    []string{"supersecret"},
			mustContain: []string{"********"},
		},
		{
			name:        "space-delimited password flag",
			cmdLine:     "/usr/bin/mysql -u root --password hunter2",
			mustOmit:    []string{"hunter2"},
			mustContain: []string{"********"},
		},
		{
			name:        "space-delimited token flag",
			cmdLine:     "/usr/bin/myapp --token abc123 start",
			mustOmit:    []string{"abc123"},
			mustContain: []string{"********"},
		},
		{
			name:        "hyphenated api-key flag",
			cmdLine:     "/app --api-key sk_live_123",
			mustOmit:    []string{"sk_live_123"},
			mustContain: []string{"********"},
		},
		{
			name:        "hyphenated access-token flag",
			cmdLine:     "/app --access-token abc123 start",
			mustOmit:    []string{"abc123"},
			mustContain: []string{"********"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, body := newInjectionMetadataReq(t, c.cmdLine)
			out := newTestForwarder(t).stripCommandLineSecrets(req, body)
			assert.NotEqual(t, body, out, "body should be modified")

			scrubbed := decodeCommandLine(t, out)
			for _, s := range c.mustOmit {
				assert.NotContains(t, scrubbed, s, "scrubbed command_line must not contain the secret %q", s)
			}
			for _, s := range c.mustContain {
				assert.Contains(t, scrubbed, s)
			}
		})
	}
}

func TestStripCommandLineSecrets_NoChangeWhenClean(t *testing.T) {
	req, body := newInjectionMetadataReq(t, "/usr/bin/python app.py --port 8080 --host 0.0.0.0")
	out := newTestForwarder(t).stripCommandLineSecrets(req, body)
	assert.Equal(t, body, out, "command line without secrets should round-trip identically")
}

func TestStripCommandLineSecrets_EmptyCommandLine(t *testing.T) {
	req, body := newInjectionMetadataReq(t, "")
	out := newTestForwarder(t).stripCommandLineSecrets(req, body)
	assert.Equal(t, body, out)
}

func TestStripCommandLineSecrets_MalformedEnvelope(t *testing.T) {
	req, _ := newInjectionMetadataReq(t, "/usr/bin/python --password=hunter2")
	bad := []byte("not json at all")
	out := newTestForwarder(t).stripCommandLineSecrets(req, bad)
	assert.Equal(t, bad, out, "malformed bodies must be forwarded unchanged so the intake can observe them")
}

func TestStripCommandLineSecrets_MalformedPayload(t *testing.T) {
	envBytes, err := json.Marshal(telemetryRequest{
		APIVersion:  "v2",
		RequestType: apmTelemetryRequestType,
		Payload:     json.RawMessage(`"this is a string, not an object"`),
	})
	assert.NoError(t, err)
	req, err := http.NewRequest("POST", apmTelemetryProxyPath, bytes.NewReader(envBytes))
	assert.NoError(t, err)
	req.Header.Set(telemetryRequestTypeHeader, apmTelemetryRequestType)
	out := newTestForwarder(t).stripCommandLineSecrets(req, envBytes)
	assert.Equal(t, envBytes, out)
}

func TestStripCommandLineSecrets_MissingPayload(t *testing.T) {
	envBytes, err := json.Marshal(telemetryRequest{
		APIVersion:  "v2",
		RequestType: apmTelemetryRequestType,
	})
	assert.NoError(t, err)
	req, err := http.NewRequest("POST", apmTelemetryProxyPath, bytes.NewReader(envBytes))
	assert.NoError(t, err)
	req.Header.Set(telemetryRequestTypeHeader, apmTelemetryRequestType)
	out := newTestForwarder(t).stripCommandLineSecrets(req, envBytes)
	assert.Equal(t, envBytes, out)
}

func TestStripCommandLineSecrets_PreservesOtherFields(t *testing.T) {
	req, body := newInjectionMetadataReq(t, "/usr/bin/python --password=hunter2 app.py")
	out := newTestForwarder(t).stripCommandLineSecrets(req, body)

	var orig, scrubbed telemetryRequest
	assert.NoError(t, json.Unmarshal(body, &orig))
	assert.NoError(t, json.Unmarshal(out, &scrubbed))

	assert.Equal(t, orig.APIVersion, scrubbed.APIVersion)
	assert.Equal(t, orig.RequestType, scrubbed.RequestType)
	assert.Equal(t, orig.TracerTime, scrubbed.TracerTime)
	assert.Equal(t, orig.RuntimeID, scrubbed.RuntimeID)
	assert.Equal(t, orig.SeqID, scrubbed.SeqID)
	assert.JSONEq(t, string(orig.Application), string(scrubbed.Application))
	assert.JSONEq(t, string(orig.Host), string(scrubbed.Host))

	var origPayload, scrubbedPayload injectionMetadata
	assert.NoError(t, json.Unmarshal(orig.Payload, &origPayload))
	assert.NoError(t, json.Unmarshal(scrubbed.Payload, &scrubbedPayload))

	assert.Equal(t, origPayload.Component, scrubbedPayload.Component)
	assert.Equal(t, origPayload.ComponentVersion, scrubbedPayload.ComponentVersion)
	assert.Equal(t, origPayload.Result, scrubbedPayload.Result)
	assert.Equal(t, origPayload.ResultReason, scrubbedPayload.ResultReason)
	assert.Equal(t, origPayload.ResultClass, scrubbedPayload.ResultClass)
	assert.Equal(t, origPayload.RuntimeID, scrubbedPayload.RuntimeID)
	assert.Equal(t, origPayload.TimestampMillis, scrubbedPayload.TimestampMillis)
	assert.Equal(t, origPayload.CreateTimeMillis, scrubbedPayload.CreateTimeMillis)
	assert.Equal(t, origPayload.Language, scrubbedPayload.Language)

	assert.NotEqual(t, origPayload.CommandLine, scrubbedPayload.CommandLine)
	assert.NotContains(t, scrubbedPayload.CommandLine, "hunter2")
}

// TestStripCommandLineSecrets_PreservesUnknownFields ensures that fields not
// modeled by telemetryRequest/injectionMetadata (e.g. added by a newer tracer
// or SSI sidecar) survive redaction instead of being dropped.
func TestStripCommandLineSecrets_PreservesUnknownFields(t *testing.T) {
	payloadBytes, err := json.Marshal(map[string]any{
		"command_line": "/usr/bin/python app.py --password=hunter2",
		"component":    "python",
		"future_field": "should be preserved",
	})
	assert.NoError(t, err)
	envBytes, err := json.Marshal(map[string]any{
		"api_version":       "v2",
		"request_type":      apmTelemetryRequestType,
		"payload":           json.RawMessage(payloadBytes),
		"another_new_field": 42,
	})
	assert.NoError(t, err)

	req, err := http.NewRequest("POST", apmTelemetryProxyPath, bytes.NewReader(envBytes))
	assert.NoError(t, err)
	req.Header.Set(telemetryRequestTypeHeader, apmTelemetryRequestType)

	out := newTestForwarder(t).stripCommandLineSecrets(req, envBytes)
	assert.NotEqual(t, envBytes, out)

	var outer map[string]json.RawMessage
	assert.NoError(t, json.Unmarshal(out, &outer))
	assert.JSONEq(t, `42`, string(outer["another_new_field"]), "unknown top-level fields must survive redaction")

	var payload map[string]json.RawMessage
	assert.NoError(t, json.Unmarshal(outer["payload"], &payload))
	assert.JSONEq(t, `"should be preserved"`, string(payload["future_field"]), "unknown payload fields must survive redaction")

	var cmdLine string
	assert.NoError(t, json.Unmarshal(payload["command_line"], &cmdLine))
	assert.NotContains(t, cmdLine, "hunter2")
	assert.Contains(t, cmdLine, "********")
}

// TestTelemetryProxy_ScrubsInjectionMetadata wires the full /telemetry/proxy
// path end-to-end: the upstream intake should receive a body whose
// command_line has been redacted.
func TestTelemetryProxy_ScrubsInjectionMetadata(t *testing.T) {
	received := make(chan []byte, 1)
	srv := assertingServer(t, func(_ *http.Request, body []byte) error {
		select {
		case received <- body:
		default:
		}
		return nil
	})

	cfg := getTestConfig(srv.URL)
	recv := newTestReceiverFromConfig(cfg)
	recv.telemetryForwarder.start()
	recv.telemetryForwarder.containerIDProvider = getTestContainerIDProvider()

	body := makeInjectionMetadataBody(t, "/usr/bin/python --password=hunter2 app.py")
	req, err := http.NewRequest("POST", "/telemetry/proxy"+apmTelemetryProxyPath, bytes.NewReader(body))
	assert.NoError(t, err)
	req.Header.Set(telemetryRequestTypeHeader, apmTelemetryRequestType)
	rec := httptest.NewRecorder()
	recv.buildMux().ServeHTTP(rec, req)
	recv.telemetryForwarder.Stop()

	assert.Equal(t, http.StatusOK, recordedStatusCode(rec))
	select {
	case got := <-received:
		assert.NotContains(t, string(got), "hunter2", "secret leaked to upstream intake")
		assert.Contains(t, decodeCommandLine(t, got), "********")
	case <-time.After(2 * time.Second):
		t.Fatal("upstream never received forwarded request")
	}
}

// TestTelemetryProxy_DoesNotScrubOtherRequestTypes ensures the gate is tight:
// payloads on the same path with a different DD-Telemetry-Request-Type are
// forwarded byte-for-byte, even when they look like they could contain
// command-line content.
func TestTelemetryProxy_DoesNotScrubOtherRequestTypes(t *testing.T) {
	received := make(chan []byte, 1)
	srv := assertingServer(t, func(_ *http.Request, body []byte) error {
		select {
		case received <- body:
		default:
		}
		return nil
	})

	cfg := getTestConfig(srv.URL)
	recv := newTestReceiverFromConfig(cfg)
	recv.telemetryForwarder.start()
	recv.telemetryForwarder.containerIDProvider = getTestContainerIDProvider()

	body := makeInjectionMetadataBody(t, "/usr/bin/python --password=hunter2 app.py")
	req, err := http.NewRequest("POST", "/telemetry/proxy"+apmTelemetryProxyPath, bytes.NewReader(body))
	assert.NoError(t, err)
	req.Header.Set(telemetryRequestTypeHeader, "app-started")
	rec := httptest.NewRecorder()
	recv.buildMux().ServeHTTP(rec, req)
	recv.telemetryForwarder.Stop()

	assert.Equal(t, http.StatusOK, recordedStatusCode(rec))
	select {
	case got := <-received:
		assert.Equal(t, body, got, "non-injection-metadata payloads must pass through verbatim")
	case <-time.After(2 * time.Second):
		t.Fatal("upstream never received forwarded request")
	}
}

// TestScrubJSONValue exercises scrubJSONValue/scrubValue directly against
// the shapes the injection-metadata metadata field could plausibly take:
// a value named directly by its JSON key, a {name, value} pair split across
// sibling keys (including inside a list), an embedded "flag=value" string,
// and data that should be left alone.
func TestScrubJSONValue(t *testing.T) {
	s := newCmdLineScrubber()

	cases := []struct {
		name        string
		in          string
		wantChanged bool
		mustOmit    []string
		mustContain []string
	}{
		{
			name:        "secret named directly by its key",
			in:          `{"password":"hunter2"}`,
			wantChanged: true,
			mustOmit:    []string{"hunter2"},
			mustContain: []string{"********"},
		},
		{
			name:        "uppercase env-var-style key",
			in:          `{"API_KEY":"abcdef0123456789"}`,
			wantChanged: true,
			mustOmit:    []string{"abcdef0123456789"},
		},
		{
			name:        "prefixed env-var-style key",
			in:          `{"DD_API_KEY":"abcdef0123456789"}`,
			wantChanged: true,
			mustOmit:    []string{"abcdef0123456789"},
		},
		{
			name:        "name/value pair split across sibling keys",
			in:          `{"env_var":"password","value":"hunter2"}`,
			wantChanged: true,
			mustOmit:    []string{"hunter2"},
			mustContain: []string{"env_var", "password", "********"},
		},
		{
			name:        "name/value pair with a service-prefixed env var name",
			in:          `{"name":"DD_API_KEY","value":"abcdef0123456789"}`,
			wantChanged: true,
			mustOmit:    []string{"abcdef0123456789"},
		},
		{
			name:        "list of name/value pairs",
			in:          `{"matched_properties":[{"name":"AUTH_TOKEN","value":"abc123"},{"name":"os","value":"linux"}]}`,
			wantChanged: true,
			mustOmit:    []string{"abc123"},
			mustContain: []string{"linux"},
		},
		{
			name:        "embedded flag=value string leaf",
			in:          `{"matched_arg":"cmd --password=hunter2"}`,
			wantChanged: true,
			mustOmit:    []string{"hunter2"},
			mustContain: []string{"********"},
		},
		{
			name:        "secret nested inside an object under a sensitive key",
			in:          `{"credentials":{"raw":"hunter2"}}`,
			wantChanged: true,
			mustOmit:    []string{"hunter2"},
			mustContain: []string{"credentials", "raw", "********"},
		},
		{
			name:        "name/value pair whose value is itself a nested object",
			in:          `{"env_var":"DD_API_KEY","value":{"raw":"abc123"}}`,
			wantChanged: true,
			mustOmit:    []string{"abc123"},
			mustContain: []string{"env_var", "DD_API_KEY", "raw", "********"},
		},
		{
			name:        "rule id and non-sensitive detected version, no scrubbing needed",
			in:          `{"rule_id":"3f29e1","detected_flavor":"musl","detected_version":"3.11.2"}`,
			wantChanged: false,
			mustContain: []string{"3f29e1", "musl", "3.11.2"},
		},
		{
			name:        "value key present but no sensitive name designator alongside it",
			in:          `{"name":"os","value":"linux"}`,
			wantChanged: false,
			mustContain: []string{"linux"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, changed, err := scrubJSONValue(json.RawMessage(c.in), s)
			assert.NoError(t, err)
			assert.Equal(t, c.wantChanged, changed)
			for _, s := range c.mustOmit {
				assert.NotContains(t, string(out), s, "scrubbed metadata must not contain the secret %q", s)
			}
			for _, s := range c.mustContain {
				assert.Contains(t, string(out), s)
			}
		})
	}
}

// TestScrubJSONValue_MalformedInput ensures a metadata blob that isn't valid
// JSON is reported as an error rather than silently dropped
func TestScrubJSONValue_MalformedInput(t *testing.T) {
	_, changed, err := scrubJSONValue(json.RawMessage(`not json`), newCmdLineScrubber())
	assert.Error(t, err)
	assert.False(t, changed)
}

// TestStripCommandLineSecrets_ScrubsMetadata verifies stripCommandLineSecrets
// end-to-end (envelope decode -> scrub -> re-encode) for a metadata field
// carrying a secret.
func TestStripCommandLineSecrets_ScrubsMetadata(t *testing.T) {
	body := makeInjectionMetadataBodyWithMetadata(t, "", json.RawMessage(`{"env_var":"password","value":"hunter2"}`))
	req, err := http.NewRequest("POST", apmTelemetryProxyPath, bytes.NewReader(body))
	assert.NoError(t, err)
	req.Header.Set(telemetryRequestTypeHeader, apmTelemetryRequestType)

	out := newTestForwarder(t).stripCommandLineSecrets(req, body)
	assert.NotEqual(t, body, out)
	assert.NotContains(t, string(out), "hunter2", "secret must not survive scrubbing")

	metadata := decodeMetadata(t, out)
	assert.Contains(t, string(metadata), "********")
	assert.Contains(t, string(metadata), "password", "the property name itself is not secret and should be preserved")
}

// TestStripCommandLineSecrets_ScrubsCommandLineAndMetadataTogether verifies
// that when both command_line and metadata need scrubbing, both patches land
// in the same output rather than only the first one taking effect.
func TestStripCommandLineSecrets_ScrubsCommandLineAndMetadataTogether(t *testing.T) {
	body := makeInjectionMetadataBodyWithMetadata(t,
		"/usr/bin/python --password=hunter2 app.py",
		json.RawMessage(`{"token":"raw-secret-value"}`),
	)
	req, err := http.NewRequest("POST", apmTelemetryProxyPath, bytes.NewReader(body))
	assert.NoError(t, err)
	req.Header.Set(telemetryRequestTypeHeader, apmTelemetryRequestType)

	out := newTestForwarder(t).stripCommandLineSecrets(req, body)
	assert.NotEqual(t, body, out)
	assert.NotContains(t, string(out), "hunter2")
	assert.NotContains(t, string(out), "raw-secret-value")
	assert.Contains(t, decodeCommandLine(t, out), "********")
	assert.Contains(t, string(decodeMetadata(t, out)), "********")
}

// TestStripCommandLineSecrets_MetadataNoChangeWhenClean ensures a metadata
// field with nothing sensitive in it round-trips identically, so the common
// case doesn't pay for a needless re-encode.
func TestStripCommandLineSecrets_MetadataNoChangeWhenClean(t *testing.T) {
	body := makeInjectionMetadataBodyWithMetadata(t, "", json.RawMessage(`{"rule_id":"3f29e1","detected_flavor":"musl"}`))
	req, err := http.NewRequest("POST", apmTelemetryProxyPath, bytes.NewReader(body))
	assert.NoError(t, err)
	req.Header.Set(telemetryRequestTypeHeader, apmTelemetryRequestType)

	out := newTestForwarder(t).stripCommandLineSecrets(req, body)
	assert.Equal(t, body, out, "metadata without secrets should round-trip identically")
}

// TestStripCommandLineSecrets_MissingMetadata ensures the absence of a
// metadata field (the common case for older/most payloads) is a no-op, not
// an error.
func TestStripCommandLineSecrets_MissingMetadata(t *testing.T) {
	body := makeInjectionMetadataBody(t, "")
	req, err := http.NewRequest("POST", apmTelemetryProxyPath, bytes.NewReader(body))
	assert.NoError(t, err)
	req.Header.Set(telemetryRequestTypeHeader, apmTelemetryRequestType)

	out := newTestForwarder(t).stripCommandLineSecrets(req, body)
	assert.Equal(t, body, out)
}

// TestTelemetryProxy_ScrubsInjectionMetadataField wires the full
// /telemetry/proxy path end-to-end: the upstream intake should receive a
// body whose metadata field has been redacted.
func TestTelemetryProxy_ScrubsInjectionMetadataField(t *testing.T) {
	received := make(chan []byte, 1)
	srv := assertingServer(t, func(_ *http.Request, body []byte) error {
		select {
		case received <- body:
		default:
		}
		return nil
	})

	cfg := getTestConfig(srv.URL)
	recv := newTestReceiverFromConfig(cfg)
	recv.telemetryForwarder.start()
	recv.telemetryForwarder.containerIDProvider = getTestContainerIDProvider()

	body := makeInjectionMetadataBodyWithMetadata(t, "", json.RawMessage(`{"token":"raw-secret-value"}`))
	req, err := http.NewRequest("POST", "/telemetry/proxy"+apmTelemetryProxyPath, bytes.NewReader(body))
	assert.NoError(t, err)
	req.Header.Set(telemetryRequestTypeHeader, apmTelemetryRequestType)
	rec := httptest.NewRecorder()
	recv.buildMux().ServeHTTP(rec, req)
	recv.telemetryForwarder.Stop()

	assert.Equal(t, http.StatusOK, recordedStatusCode(rec))
	select {
	case got := <-received:
		assert.NotContains(t, string(got), "raw-secret-value", "secret leaked to upstream intake")
		assert.Contains(t, string(decodeMetadata(t, got)), "********")
	case <-time.After(2 * time.Second):
		t.Fatal("upstream never received forwarded request")
	}
}
