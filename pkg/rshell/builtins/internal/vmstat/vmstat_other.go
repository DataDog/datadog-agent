// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux && !darwin

package vmstat

import "context"

// readImpl returns ErrNotSupported on platforms without a backend
// (Windows and anything else). Mirrors diskstats_other.go.
func readImpl(_ context.Context, _ string) (Stats, error) {
	return Stats{}, ErrNotSupported
}
