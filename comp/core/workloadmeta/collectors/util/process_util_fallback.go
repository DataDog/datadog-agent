// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package util

// LocalProcessCollectorIsEnabled returns whether the local process collector is enabled
// based on agent flavor and config values. This prevents any conflict between the collectors
// and unnecessary data collection. Always returns false outside of linux.
func LocalProcessCollectorIsEnabled() bool {
	return false
}
