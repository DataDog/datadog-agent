// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package diskv2 provides Disk Check.
package diskv2

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/benbjohnson/clock"
	gopsutil_disk "github.com/shirou/gopsutil/v4/disk"
	"github.com/spf13/afero"
)

// Test helpers

// WithClock sets a custom clock on the Check and returns the updated Check.
func WithClock(c check.Check, clock clock.Clock) check.Check {
	c.(*Check).clock = clock
	return c
}

// WithDiskPartitionsWithContext sets a diskPartitionsWithContext call on the Check and returns the updated Check.
func WithDiskPartitionsWithContext(c check.Check, f func(context.Context, bool) ([]gopsutil_disk.PartitionStat, error)) check.Check {
	c.(*Check).diskPartitionsWithContext = f
	return c
}

// WithDiskUsage sets a diskUsage call on the Check and returns the updated Check.
func WithDiskUsage(c check.Check, f func(string) (*gopsutil_disk.UsageStat, error)) check.Check {
	c.(*Check).diskUsage = f
	return c
}

// WithDiskIOCounters sets a diskIOCounters call on the Check and returns the updated Check.
func WithDiskIOCounters(c check.Check, f func(...string) (map[string]gopsutil_disk.IOCountersStat, error)) check.Check {
	c.(*Check).diskIOCounters = f
	return c
}

// WithFs sets a custom clock on the Check and returns the updated Check.
func WithFs(c check.Check, fs afero.Fs) check.Check {
	c.(*Check).fs = fs
	return c
}

// WithStat sets a statFn call on the Check and returns the updated Check.
func WithStat(c check.Check, f func(string) (StatT, error)) check.Check {
	c.(*Check).statFn = f
	return c
}
