// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock defines useful entities for mocking the flare profiler and its operations
package mock

import (
	"net/http"
)

// NewMockHandler provides a standard handler that will mimic the endpoints the profiler will scan for pprof data
// Can be injected directly in to an httptest server
func NewMockHandler() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/debug/pprof/heap":
			w.Write([]byte("heap_profile"))
		case "/debug/pprof/profile":
			time := r.URL.Query()["seconds"][0]
			w.Write([]byte(time + "_sec_cpu_pprof"))
		case "/debug/pprof/mutex":
			w.Write([]byte("mutex"))
		case "/debug/pprof/block":
			w.Write([]byte("block"))
		case "/debug/stats": // only for system-probe
			w.WriteHeader(200)
		case "/debug/pprof/trace":
			w.Write([]byte("trace"))
		default:
			w.WriteHeader(500)
		}
	})
}
