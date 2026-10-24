// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

package procnetroute

import (
	"context"
	"errors"
)

// readRoutes is the non-Linux stub; routing table reading is Linux-only.
func readRoutes(_ context.Context, _ string) ([]Route, error) {
	return nil, errors.New("route table reading is not supported on this platform")
}
