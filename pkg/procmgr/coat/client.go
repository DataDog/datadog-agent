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
	// Describe returns the full detail for one process, including the fields List omits.
	Describe(ctx context.Context, nameOrUUID string) (ProcessSnapshot, error)
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

// serviceSweepReserve is the part of a collection budget held back from the dd-procmgrd calls so the
// per-service supervisor checks that follow them still have time to run.
//
// Those checks are the only part of a snapshot that does not go through dd-procmgrd, which makes them
// what is left when the daemon is itself the problem. They share the collection context, and a
// context cannot outlive an expired parent, so without a reserve a daemon that hangs long enough to
// exhaust the budget makes every "systemctl is-active" fail on arrival. Every service would then
// report management_mode "none", which claims no supervisor owns it rather than admitting we could
// not tell.
//
// One value serves both callers when the budget is large enough for it. When the caller has given us
// no more than the reserve, the carve-out is skipped: the fleet daemon, for example, polls Collect
// with a two-second deadline, and subtracting the full reserve there would expire the daemon calls
// on arrival and make every procmgr-managed service report unknown.
const serviceSweepReserve = 2 * time.Second

// daemonPhaseContext bounds the dd-procmgrd calls so they cannot spend a whole collection budget,
// leaving serviceSweepReserve of it for the local checks that run afterwards.
//
// Derived from the collection context rather than the caller's, so the reserve is carved out of the
// budget collection already agreed to and any margin the caller set aside is still respected. When
// the remaining budget is no larger than the reserve, the carve-out is skipped and the daemon calls
// keep the full budget: failing them on arrival would hide procmgr ownership, which is worse than
// a service sweep that may itself run short.
func daemonPhaseContext(collection context.Context) (context.Context, context.CancelFunc) {
	deadline, ok := collection.Deadline()
	if !ok {
		return context.WithCancel(collection)
	}
	remaining := time.Until(deadline)
	if remaining <= serviceSweepReserve {
		return context.WithCancel(collection)
	}
	return context.WithDeadline(collection, deadline.Add(-serviceSweepReserve))
}

func newDefaultClient() Client {
	return newGRPCClient(procmgrSocketPath())
}
