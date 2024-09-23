// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && !linux

package tracer

// IsTracerSupportedByOS returns whether or not the current kernel version supports tracer functionality
// along with some context on why it's not supported
func IsTracerSupportedByOS(_ []string) (bool, string) {
	return verifyOSVersion(0, "", nil)
}

func verifyOSVersion(_ uint32, _ string, _ []string) (bool, string) {
	return false, "unsupported architecture"
}
