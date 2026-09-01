// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"context"
)

// experimentSupervisor watches a running version experiment and takes the host back to stable when
// the experiment fails or outstays its deadline.
//
// It exists in the daemon rather than in the installer because the installer is a short-lived
// process: it returns as soon as the experiment's jobs are up, and there is nobody left to notice
// that one of them died two minutes later. The daemon is the only thing that is still there.
//
// Only macOS supplies one. On Linux and Windows the platform supervises the experiment itself —
// systemd and the Windows service manager both stop an experiment unit that fails and there is a
// timer alongside it — so there is nothing for the daemon to do and the interface is nil.
type experimentSupervisor interface {
	// Reconcile is called once at daemon start, to reckon with whatever the host was doing
	// before this process existed: a reboot in the middle of an experiment, or a daemon that was
	// killed and restarted.
	Reconcile(ctx context.Context) error
	// Tick is called from the daemon's existing ticker. It is the only place the deadline is
	// checked and the only place an experiment job's exit is noticed.
	Tick(ctx context.Context) error
}
