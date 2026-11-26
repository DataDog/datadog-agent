// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"sync/atomic"
	"time"
)

// NvmlReleaseLeaseTTL is the default lease duration, used when a push does
// not carry one (3x a default check interval). The core agent sends a TTL
// derived from its actual check interval, so the lease always tolerates a
// few missed pushes while still self-healing: a core agent that dies
// mid-window leaves this probe released at most one lease long.

// NvmlReleaseLeaseMaxTTL is the sanity bound on pushed lease durations. It
// must stay above the longest plausible check interval: the lease has to
// cover the renewal cadence (3 runs), and clamping below it would let
// system-probe re-acquire NVML mid-window. 24h bounds garbage values while
// keeping correctness for any sane interval.
const NvmlReleaseLeaseMaxTTL = 24 * time.Hour
const NvmlReleaseLeaseTTL = 30 * time.Second

// NvmlReleaseLease is the lease the core agent holds on this probe's NVML
// release: system-probe is a separate process and an independent NVML client,
// so the core agent pushes renewals over the system-probe HTTP socket while
// the window is open, and the lease expiring (or a released=false push) ends
// it. Every failure mode — core agent crash, check reload, socket blips —
// just stops the renewals.
type NvmlReleaseLease struct {
	// deadlineNanos is the Unix-nano deadline of the current lease; 0 means
	// no window.
	deadlineNanos atomic.Int64
}

// Renew extends the lease by ttl from now.
func (l *NvmlReleaseLease) Renew(ttl time.Duration) {
	l.deadlineNanos.Store(time.Now().Add(ttl).UnixNano())
}

// Clear ends the window immediately (released=false push).
func (l *NvmlReleaseLease) Clear() {
	l.deadlineNanos.Store(0)
}

// Held reports whether the release lease is currently held.
func (l *NvmlReleaseLease) Held() bool {
	deadline := l.deadlineNanos.Load()
	return deadline != 0 && time.Now().UnixNano() < deadline
}

// Hold renews the lease for the pushed duration, clamping implausible values:
// non-positive to the default TTL, oversized to the max (a lease shorter than
// its renewal cadence would expire between renewals).
func (l *NvmlReleaseLease) Hold(ttl time.Duration) {
	switch {
	case ttl <= 0:
		ttl = NvmlReleaseLeaseTTL
	case ttl > NvmlReleaseLeaseMaxTTL:
		ttl = NvmlReleaseLeaseMaxTTL
	}
	l.Renew(ttl)
}
