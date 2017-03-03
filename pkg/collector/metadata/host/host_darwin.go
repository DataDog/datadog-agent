package host

import (
	"runtime"

	"github.com/shirou/gopsutil/host"
)

type osVersion struct {
	Release     string    `json:"release"`
	Versioninfo [3]string `json:"versioninfo"`
	Machine     string    `json:"machine"`
}

func fillOsVersion(stats *systemStats, info *host.InfoStat) {
	stats.Macver = osVersion{
		Release:     info.PlatformVersion,
		Versioninfo: [3]string{"", "", ""},
		Machine:     runtime.GOARCH,
	}
}
