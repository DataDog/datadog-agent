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

// Platform is the string describing the Linux distribution (ubuntu, debian, fedora, etc.)
var Platform = funcs.Memoize(func() (string, error) {
	platform, _, _, err := gopsutilhost.PlatformInformation()
	return platform, err
})
