// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package serializers defines functions aiming to serialize events
package serializers

// nolint: deadcode, unused
func getUint64Pointer(i *uint64) *uint64 {
	if *i == 0 {
		return nil
	}
	return i
}

// nolint: deadcode, unused
func getUint32Pointer(i *uint32) *uint32 {
	if *i == 0 {
		return nil
	}
	return i
}
