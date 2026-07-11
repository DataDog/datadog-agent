// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package coat

import (
	"context"
	"time"
)

// ProcmgrSession is an open gRPC session to dd-procmgrd. Call Disconnect when finished.
type ProcmgrSession interface {
	Status(ctx context.Context) (DaemonSnapshot, error)
	List(ctx context.Context) (map[string]ProcessSnapshot, error)
	Disconnect() error
}

// Client opens sessions to dd-procmgrd.
type Client interface {
	Connect(ctx context.Context) (ProcmgrSession, error)
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
