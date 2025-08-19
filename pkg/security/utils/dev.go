// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

// Mkdev returns the representation of a device
// use the kernel algorithm, the golang unix.Mkdev function bring inconsistency between representations of device
// https://elixir.bootlin.com/linux/v6.4.9/source/include/linux/kdev_t.h#L12
func Mkdev(major uint32, minor uint32) uint32 {
	const minorBits = 20
	return (major << minorBits) | minor
}
