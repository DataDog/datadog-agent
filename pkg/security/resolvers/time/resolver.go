// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package time

import (
	"fmt"
	"time"

	"github.com/DataDog/gopsutil/host"
	"golang.org/x/sys/unix"
)

// Resolver converts kernel monotonic timestamps to absolute times
type Resolver struct {
	bootTime time.Time
}

// NewResolver returns a new time resolver
func NewResolver() (*Resolver, error) {
	bt, err := host.BootTime()
	if err != nil {
		return nil, err
	}

	tr := Resolver{
		bootTime: time.Unix(int64(bt), 0),
	}
	return &tr, nil
}

func (tr *Resolver) getUptimeOffset() (time.Duration, error) {
	upTime := new(unix.Timespec)
	err := unix.ClockGettime(unix.CLOCK_MONOTONIC, upTime)
	if err != nil {
		return 0, fmt.Errorf("couldn't get system up time: %w", err)
	}
	return time.Since(tr.bootTime) - time.Duration(upTime.Nano()), nil
}

// ResolveMonotonicTimestamp converts a kernel monotonic timestamp to an absolute time
func (tr *Resolver) ResolveMonotonicTimestamp(timestamp uint64) time.Time {
	if timestamp > 0 {
		// ignore uptime resolution failure: default back to previous behavior
		offset, _ := tr.getUptimeOffset()
		return tr.bootTime.Add(time.Duration(timestamp) + offset)
	}
	return time.Time{}
}

// ApplyBootTime return the time re-aligned from the boot time
func (tr *Resolver) ApplyBootTime(timestamp time.Time) time.Time {
	if !timestamp.IsZero() {
		// ignore uptime resolution failure: default back to previous behavior
		offset, _ := tr.getUptimeOffset()
		return timestamp.Add(time.Duration(tr.bootTime.UnixNano()) + offset)
	}
	return time.Time{}
}

// ComputeMonotonicTimestamp converts an absolute time to a kernel monotonic timestamp
func (tr *Resolver) ComputeMonotonicTimestamp(timestamp time.Time) int64 {
	if !timestamp.IsZero() {
		// ignore uptime resolution failure: default back to previous behavior
		offset, _ := tr.getUptimeOffset()
		return timestamp.Sub(tr.bootTime.Add(offset)).Nanoseconds()
	}
	return 0
}
