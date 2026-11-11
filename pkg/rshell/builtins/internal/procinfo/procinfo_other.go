// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux && !darwin && !windows

package procinfo

import (
	"context"
	"errors"
)

var errUnsupported = errors.New("not supported on this platform")

func listAll(_ context.Context, _ string, _ Metrics) ([]ProcInfo, error) {
	return nil, errUnsupported
}

func getSession(_ context.Context, _ string, _ Metrics) ([]ProcInfo, error) {
	return nil, errUnsupported
}

func getByPIDs(_ context.Context, _ string, _ []int, _ Metrics) ([]ProcInfo, error) {
	return nil, errUnsupported
}
