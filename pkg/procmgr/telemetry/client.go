// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetry

import (
	"context"
	"time"
)

// Client queries dd-procmgrd state.
type Client interface {
	DaemonStatus(ctx context.Context) (DaemonSnapshot, error)
	ListProcesses(ctx context.Context) (map[string]ProcessSnapshot, error)
}

const clientTimeout = 5 * time.Second

func clientContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, clientTimeout)
}

func newDefaultClient() Client {
	return newGRPCClient(procmgrSocketPath())
}
