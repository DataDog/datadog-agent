package host

import (
	"runtime"

	"github.com/shirou/gopsutil/host"
)

type osVersion [3]interface{}

func fillOsVersion(stats *systemStats, info *host.InfoStat) {
	stats.Macver = osVersion{info.PlatformVersion, [3]string{"", "", ""}, runtime.GOARCH}
}
