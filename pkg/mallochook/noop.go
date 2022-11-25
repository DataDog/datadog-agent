// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package mallochook

// Supported returns true if the current mallochook is supported on the current platform
func Supported() bool {
	return false
}

// GetStats returns a snapshot of memory allocation statistics
func GetStats() Stats {
	return Stats{}
}
