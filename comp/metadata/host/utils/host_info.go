// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Portions of this code are taken from the gopsutil project
// https://github.com/shirou/gopsutil .  This code is licensed under the New BSD License
// copyright WAKAYAMA Shirou, and the gopsutil contributors

// Package utils TODO comment
package utils

import "github.com/DataDog/datadog-agent/pkg/util/cache"

var (
	cacheKey = cache.BuildAgentKey("host", "utils", "hostInfo")
)

// GetPlatformName returns the name of the current platform
func GetPlatformName() string {
	return GetInformation().Platform
}

// GetKernelVersion returns the kernel version
func GetKernelVersion() string {
	return GetInformation().KernelVersion
}
