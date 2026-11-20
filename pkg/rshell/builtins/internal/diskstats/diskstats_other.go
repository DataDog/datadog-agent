// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux && !darwin

package diskstats

import "context"

// listImpl returns ErrNotSupported on platforms without a backend.
func listImpl(_ context.Context, _ FilterFunc) ([]Mount, error) {
	return nil, ErrNotSupported
}
