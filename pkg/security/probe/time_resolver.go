// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"time"

	"github.com/DataDog/gopsutil/host"
)

// TimeResolver converts kernel monotonic timestamps to absolute times
type TimeResolver struct {
	bootTime time.Time
}

// NewTimeResolver returns a new time resolver
func NewTimeResolver() (*TimeResolver, error) {
	bt, err := host.BootTime()
	if err != nil {
		return nil, err
	}
	tr := TimeResolver{
		bootTime: time.Unix(int64(bt), 0),
	}
	return &tr, nil
}

// ResolveMonotonicTimestamp converts a kernel monotonic timestamp to an absolute time
func (tr *TimeResolver) ResolveMonotonicTimestamp(timestamp uint64) time.Time {
	if timestamp > 0 {
		return tr.bootTime.Add(time.Duration(timestamp))
	}
	return time.Time{}
}

// ApplyBootTime return the time re-aligned from the boot time
func (tr *TimeResolver) ApplyBootTime(timestamp time.Time) time.Time {
	if !timestamp.IsZero() {
		return timestamp.Add(time.Duration(tr.bootTime.UnixNano()))
	}
	return time.Time{}
}

// ComputeMonotonicTimestamp converts an absolute time to a kernel monotonic timestamp
func (tr *TimeResolver) ComputeMonotonicTimestamp(timestamp time.Time) int64 {
	if !timestamp.IsZero() {
		return timestamp.Sub(tr.bootTime).Nanoseconds()
	}
	return 0
}
