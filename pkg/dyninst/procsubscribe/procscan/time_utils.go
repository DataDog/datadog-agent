// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procscan

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

// CLK_TCK is a constant on Linux for all architectures except alpha and ia64.
// See e.g.
// https://git.musl-libc.org/cgit/musl/tree/src/conf/sysconf.c#n30
// https://github.com/containerd/cgroups/pull/12
// https://lore.kernel.org/lkml/agtlq6$iht$1@penguin.transmeta.com/
//
// See https://github.com/tklauser/go-sysconf/blob/e2b5de3c/sysconf_linux.go#L19-L24
const clkTck = 100

// ticks is a type for representing time in ticks.
type ticks uint64

// nowTicks returns the current time since boot, expressed in USER_HZ ticks.
// It prefers CLOCK_BOOTTIME (includes suspend), and falls back to CLOCK_MONOTONIC
// if BOOTTIME isn't available on the running kernel.
func nowTicks() (ticks, error) {

	// Try CLOCK_BOOTTIME first.
	ts, err := clockGettime(unix.CLOCK_BOOTTIME)
	if err != nil {
		// Fallback: CLOCK_MONOTONIC (excludes suspend). Callers comparing
		// with /proc/<pid>/stat starttime should prefer BOOTTIME, but
		// MONOTONIC is better than failing outright on older systems.
		ts, err = clockGettime(unix.CLOCK_MONOTONIC)
		if err != nil {
			return ticks(0), fmt.Errorf("clock_gettime fallback failed: %w", err)
		}
	}

	// Convert timespec to ticks with integer math.
	// Clock ticks are at 100 Hz (CLK_TCK = 100), so:
	// - seconds to ticks: multiply by 100
	// - nanoseconds to ticks: multiply by 100, divide by 1 billion
	secTicks := uint64(ts.Sec) * clkTck
	nsecTicks := (uint64(ts.Nsec) * clkTck) / 1_000_000_000

	return ticks(secTicks + nsecTicks), nil
}

// clockGettime wraps unix.ClockGettime and normalizes EINTR.
func clockGettime(clockID int32) (unix.Timespec, error) {
	var ts unix.Timespec
	if err := unix.ClockGettime(clockID, &ts); err != nil {
		// Some platforms may return EINVAL for unsupported clocks.
		// Treat any error as non-retryable here; caller can decide on fallback.
		return unix.Timespec{}, err
	}
	// Sanity check: ensure non-negative.
	if ts.Sec < 0 || ts.Nsec < 0 {
		return unix.Timespec{}, errors.New("negative timespec from clock_gettime")
	}
	return ts, nil
}
