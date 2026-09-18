// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package blackhole implements a stateless Datadog HTTP intake sink. It does not
// decompress, retain, forward, or log payloads, credentials, paths or headers.
package blackhole

import (
	"io"
	"net/http"

	process "github.com/DataDog/agent-payload/v5/process"
)

// MaxBodyBytes bounds work per request, including chunked requests. Bodies are
// streamed to io.Discard with constant memory, not buffered or decompressed.
const MaxBodyBytes = 64 << 20

// Handler returns an independent sink with no store or query API. RC is not
// supported: callers must disable it in the Agent's configuration.
func Handler() http.Handler {
	// Precompute a constant acknowledgement once per handler, independently of
	// request data. Zero active clients never enables realtime process collection.
	ack, err := process.EncodeMessage(process.Message{
		Header: process.MessageHeader{Version: process.MessageV3, Encoding: process.MessageEncodingProtobuf, Type: process.TypeResCollector},
		Body:   &process.ResCollector{Status: &process.CollectorStatus{}},
	})
	if err != nil {
		panic("invalid static process acknowledgement")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v0.1/configurations" || r.URL.Path == "/api/v0.1/org" || r.URL.Path == "/api/v0.1/status" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodPost, http.MethodPut:
			if _, err := io.Copy(io.Discard, http.MaxBytesReader(w, r.Body, MaxBodyBytes)); err != nil {
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				return
			}
			switch r.URL.Path {
			case "/api/v1/collector", "/api/v1/discovery", "/api/v1/container", "/api/v1/connections", "/api/v1/orchestrator", "/api/v2/orch", "/api/v2/orchmanif":
				w.Header().Set("Content-Type", "application/x-protobuf")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(ack)
			default:
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, `{}`)
			}
		case http.MethodGet, http.MethodHead:
			if r.URL.Path != "/api/v1/validate" && r.URL.Path != "/health" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			if r.Method != http.MethodHead {
				_, _ = io.WriteString(w, `{"valid":true}`)
			}
		default:
			w.Header().Set("Allow", "POST, PUT, GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}
