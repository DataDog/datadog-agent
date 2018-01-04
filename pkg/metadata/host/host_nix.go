// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build linux freebsd netbsd openbsd solaris dragonfly

package host

import "github.com/shirou/gopsutil/host"

type osVersion [3]string

func fillOsVersion(stats *systemStats, info *host.InfoStat) {
	stats.Nixver = osVersion{info.PlatformFamily, info.PlatformVersion, ""}
}
