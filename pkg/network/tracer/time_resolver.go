// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package tracer

import (
	"fmt"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/gopsutil/host"
)

// TimeResolver converts kernel monotonic timestamps to absolute times
type TimeResolver struct {
	bootTime    time.Time
	suspendTime uint64
}

// NewTimeResolver returns a new time resolver
func NewTimeResolver() (*TimeResolver, error) {
	bt, err := host.BootTime()
	if err != nil {
		return nil, err
	}

	tr := &TimeResolver{
		bootTime: time.Unix(int64(bt), 0),
	}

	if err = tr.Sync(); err != nil {
		return nil, fmt.Errorf("could not determine suspend time for system: %w", err)
	}

	return tr, nil
}

// Sync computes the current suspend time for the system
func (tr *TimeResolver) Sync() error {
	if tr == nil {
		return nil
	}

	var monotonic unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &monotonic); err != nil {
		return err
	}

	tr.suspendTime = uint64(time.Since(tr.bootTime) - time.Duration(monotonic.Nano()))
	return nil
}

// ResolveMonotonicTimestamp converts a kernel monotonic timestamp to an absolute time, nanoseconds since Unix epoch
func (tr *TimeResolver) ResolveMonotonicTimestamp(timestamp uint64) uint64 {
	if tr == nil {
		return timestamp
	}

	return uint64(tr.bootTime.UnixNano()) + timestamp + tr.suspendTime
}
