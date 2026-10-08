// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

// benchInjectionMetadataBody builds an injection-metadata envelope of the
// shape the SSI forwarder sends, with the given command line and metadata.
func benchInjectionMetadataBody(b *testing.B, cmdLine string, metadata json.RawMessage) []byte {
	b.Helper()
	payload, err := json.Marshal(injectionMetadata{
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
	if err != nil {
		b.Fatal(err)
	}
	body, err := json.Marshal(telemetryRequest{
		APIVersion:  "v2",
		RequestType: apmTelemetryRequestType,
		TracerTime:  1746722643,
		RuntimeID:   "f81d4fae-7dec-11d0-a765-00a0c91e6bf6",
		SeqID:       1,
		Application: json.RawMessage(`{"service_name":"test_service","language_name":"python","tracer_version":"1.0.0"}`),
		Host:        json.RawMessage(`{"hostname":"test_host","container_id":"test_cid"}`),
		Payload:     payload,
	})
	if err != nil {
		b.Fatal(err)
	}
	return body
}

func benchInjectionMetadataReq(requestType string) *http.Request {
	req := &http.Request{
		Method: "POST",
		URL:    &url.URL{Path: apmTelemetryProxyPath},
		Header: http.Header{},
	}
	req.Header.Set(telemetryRequestTypeHeader, requestType)
	return req
}

// BenchmarkStripCommandLineSecrets covers the four shapes the proxy sees: a
// request the gate rejects, a clean payload that must round-trip unchanged, a
// payload whose command_line needs redacting, and one where command_line and
// the free-form metadata field both do.
func BenchmarkStripCommandLineSecrets(b *testing.B) {
	richMetadata := json.RawMessage(`{"rule_id":"3f29e1","detected_flavor":"musl","detected_version":"3.11.2",` +
		`"matched_properties":[{"name":"PYTHONPATH","value":"/app"},{"name":"HOME","value":"/root"}]}`)
	secretMetadata := json.RawMessage(`{"rule_id":"3f29e1","detected_flavor":"musl",` +
		`"matched_properties":[{"name":"AUTH_TOKEN","value":"abc123"},{"name":"HOME","value":"/root"}]}`)

	cases := []struct {
		name        string
		requestType string
		cmdLine     string
		metadata    json.RawMessage
	}{
		{
			name:        "gate_miss",
			requestType: "app-started",
			cmdLine:     "/usr/bin/python app.py --password=hunter2",
		},
		{
			name:        "clean",
			requestType: apmTelemetryRequestType,
			cmdLine:     "/usr/bin/python app.py --port 8080 --host 0.0.0.0",
			metadata:    richMetadata,
		},
		{
			name:        "command_line_secret",
			requestType: apmTelemetryRequestType,
			cmdLine:     "/usr/bin/python app.py --password=hunter2 --port 8080",
			metadata:    richMetadata,
		},
		{
			name:        "command_line_and_metadata_secret",
			requestType: apmTelemetryRequestType,
			cmdLine:     "/usr/bin/python app.py --password=hunter2 --port 8080",
			metadata:    secretMetadata,
		},
	}

	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			f := newTestReceiverFromConfig(newTestReceiverConfig()).telemetryForwarder
			req := benchInjectionMetadataReq(c.requestType)
			body := benchInjectionMetadataBody(b, c.cmdLine, c.metadata)
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				out := f.stripCommandLineSecrets(req, body)
				if len(out) == 0 {
					b.Fatal("empty output")
				}
			}
		})
	}
}
