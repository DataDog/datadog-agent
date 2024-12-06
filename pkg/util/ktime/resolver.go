// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ktime holds time related files
package ktime

import (
	"time"

	_ "unsafe" // unsafe import to call nanotime() which should be 2x quick than time.Now()

	"github.com/shirou/gopsutil/v3/host"
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

//go:noescape
//go:linkname nanotime runtime.nanotime
func nanotime() int64

func (tr *Resolver) getUptimeOffset() time.Duration {
	return time.Since(tr.bootTime) - time.Duration(nanotime())
}

// ResolveMonotonicTimestamp converts a kernel monotonic timestamp to an absolute time
func (tr *Resolver) ResolveMonotonicTimestamp(timestamp uint64) time.Time {
	if timestamp > 0 {
		offset := tr.getUptimeOffset()
		return tr.bootTime.Add(time.Duration(timestamp) + offset)
	}
	return time.Time{}
}

// ApplyBootTime return the time re-aligned from the boot time
func (tr *Resolver) ApplyBootTime(timestamp time.Time) time.Time {
	if !timestamp.IsZero() {
		offset := tr.getUptimeOffset()
		return timestamp.Add(time.Duration(tr.bootTime.UnixNano()) + offset)
	}
	return time.Time{}
}

// ComputeMonotonicTimestamp converts an absolute time to a kernel monotonic timestamp
func (tr *Resolver) ComputeMonotonicTimestamp(timestamp time.Time) int64 {
	if !timestamp.IsZero() {
		return timestamp.Sub(tr.GetBootTime()).Nanoseconds()
	}
	return 0
}

// GetBootTime returns boot time
func (tr *Resolver) GetBootTime() time.Time {
	return tr.bootTime.Add(tr.getUptimeOffset())
}
