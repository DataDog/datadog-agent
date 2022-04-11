// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package host

import (
	"runtime"

	"github.com/shirou/gopsutil/v3/host"
)

type osVersion [3]interface{}

const osName = runtime.GOOS

func fillOsVersion(stats *systemStats, info *host.InfoStat) {
	stats.Macver = osVersion{info.PlatformVersion, [3]string{"", "", ""}, runtime.GOARCH}
}
