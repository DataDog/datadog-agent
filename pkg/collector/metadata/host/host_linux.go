package host

import "github.com/shirou/gopsutil/host"

type osVersion [3]string

func fillOsVersion(stats *systemStats, info *host.InfoStat) {
	stats.Nixver = osVersion{info.PlatformFamily, info.PlatformVersion, ""}
}
