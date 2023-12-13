// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package system

const (
	defaultCPUCountUnitTest = 3
)

func init() {
	hostCPUCount.Store(defaultCPUCountUnitTest)
}

// SetHostCPUCount sets the host CPU count to the given value.
// (Unit tests only)
func SetHostCPUCount(count int) {
	hostCPUCount.Store(int64(count))
}

// ResetHostCPUCount resets the host CPU count to the default value.
// (Unit tests only)
func ResetHostCPUCount() {
	SetHostCPUCount(defaultCPUCountUnitTest)
}
