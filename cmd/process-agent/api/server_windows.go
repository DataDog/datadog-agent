// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package api

import "net/http"

// registerPlatformHandlers registers process-agent routes that exist only on this platform.
func registerPlatformHandlers(_ APIServerDeps, r *http.ServeMux) {
	r.HandleFunc("GET /pid/{pid}/sid", sidForPIDHandler)
}
