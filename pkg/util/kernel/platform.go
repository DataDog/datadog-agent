// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kernel

import (
	gopsutilhost "github.com/shirou/gopsutil/v3/host"

	"github.com/DataDog/datadog-agent/pkg/util/funcs"
)

type platformInfo struct {
	platform string
	family   string
	version  string
}

// Platform is the string describing the Linux distribution (ubuntu, debian, fedora, etc.)
var Platform = funcs.Memoize(func() (string, error) {
	pi, err := platformInformation()
	return pi.platform, err
})

// PlatformVersion is the string describing the platform version (`22.04` for Ubuntu jammy, etc.)
var PlatformVersion = funcs.Memoize(func() (string, error) {
	pi, err := platformInformation()
	return pi.version, err
})

var platformInformation = funcs.Memoize(func() (platformInfo, error) {
	platform, family, version, err := gopsutilhost.PlatformInformation()
	return platformInfo{platform, family, version}, err
})
