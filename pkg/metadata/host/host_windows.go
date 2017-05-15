package host

import "github.com/shirou/gopsutil/host"

type osVersion [2]string

func fillOsVersion(stats *systemStats, info *host.InfoStat) {
	// TODO
	stats.Winver = osVersion{info.PlatformFamily, info.PlatformVersion}
}
