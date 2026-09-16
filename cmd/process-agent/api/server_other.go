// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package api

import "net/http"

// registerPlatformHandlers is a no-op on this platform: there are no platform-only routes here yet.
func registerPlatformHandlers(_ APIServerDeps, _ *http.ServeMux) {}
