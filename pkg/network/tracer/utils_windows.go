// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package tracer

// IsTracerSupportedByOS returns whether or not the current kernel version supports tracer functionality
// along with some context on why it's not supported
func IsTracerSupportedByOS(exclusionList []string) (bool, string) {
	return verifyOSVersion(0, "", nil)
}

// TODO Determine which windows versions we will support, and potentially remove kernelCode from parameters list
func verifyOSVersion(kernelCode uint32, platform string, exclusionList []string) (bool, string) {
	return true, ""
}
