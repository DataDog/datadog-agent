// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux && !windows && !darwin

package procmaps

import "context"

// readImpl returns ErrNotSupported on platforms without a backend
// (Linux, Windows, and macOS all have one — see procmaps_darwin.go,
// procmaps_windows.go, and the Linux implementation).
func readImpl(_ context.Context, _ string, _ int, _ bool) (string, []Mapping, error) {
	return "", nil, ErrNotSupported
}
