// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package procutil

import "time"

// WithReturnZeroPermStats configures whether StatsWithPermByPID() returns StatsWithPerm that
// has zero values on all fields
func WithReturnZeroPermStats(enabled bool) Option {
	return func(p Probe) {}
}

// WithPermission configures if process collection should fetch fields
// that require elevated permission or not
func WithPermission(elevatedPermissions bool) Option {
	return func(p Probe) {}
}

// WithBootTimeRefreshInterval configures the boot time refresh interval
func WithBootTimeRefreshInterval(bootTimeRefreshInterval time.Duration) Option {
	return func(p Probe) {}
}
