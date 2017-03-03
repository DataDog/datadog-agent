package host

import (
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check/py"
	"github.com/DataDog/datadog-agent/pkg/collector/metadata"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/host"

	log "github.com/cihub/seelog"
)

const packageCachePrefix = "host"

// GetPayload builds a metadata payload every time is called.
// Some data is collected only once, some is cached, some is collected at every call.
func GetPayload(hostname string) *Payload {
	meta := getMeta()
	meta.Hostname = hostname

	return &Payload{
		Os:               runtime.GOOS,
		PythonVersion:    getPythonVersion(),
		InternalHostname: hostname,
		UUID:             getHostInfo().HostID,
		SytemStats:       getSystemStats(),
		Meta:             meta,
	}
}

func getSystemStats() *systemStats {
	var stats *systemStats
	key := buildKey("systemStats")
	if x, found := util.Cache.Get(key); found {
		stats = x.(*systemStats)
	} else {
		cpuInfo := getCPUInfo()
		hostInfo := getHostInfo()

		stats = &systemStats{
			Machine:   runtime.GOARCH,
			Platform:  runtime.GOOS,
			Processor: cpuInfo.ModelName,
			CPUCores:  cpuInfo.Cores,
			Pythonv:   strings.Split(getPythonVersion(), " ")[0],
		}

		// fill the platform dependent bits of info
		fillOsVersion(stats, hostInfo)
		util.Cache.Set(key, stats, util.NoExpiration)
	}

	return stats
}

func getPythonVersion() string {
	var pythonVersion string
	key := buildKey("python")
	if x, found := util.Cache.Get(key); found {
		pythonVersion = x.(string)
	} else {
		pythonVersion = py.GetInterpreterVersion()
		util.Cache.Set(key, pythonVersion, util.NoExpiration)
	}

	return pythonVersion
}

// getCPUInfo returns InfoStat for the first CPU gopsutil found
func getCPUInfo() *cpu.InfoStat {
	key := buildKey("cpuInfo")
	if x, found := util.Cache.Get(key); found {
		return x.(*cpu.InfoStat)
	}

	i, err := cpu.Info()
	if err != nil {
		// don't cache and return zero value
		log.Errorf("failed to retrieve cpu info: %s", err)
		return &cpu.InfoStat{}
	}
	info := &i[0]
	util.Cache.Set(key, info, util.NoExpiration)
	return info
}

func getHostInfo() *host.InfoStat {
	key := buildKey("hostInfo")
	if x, found := util.Cache.Get(key); found {
		return x.(*host.InfoStat)
	}

	info, err := host.Info()
	if err != nil {
		// don't cache and return zero value
		log.Errorf("failed to retrieve host info: %s", err)
		return &host.InfoStat{}
	}
	util.Cache.Set(key, info, util.NoExpiration)
	return info
}

func getMeta() *meta {
	hostname, _ := os.Hostname()
	tzname, _ := time.Now().Zone()

	return &meta{
		SocketHostname: hostname,
		Timezones:      []string{tzname},
		SocketFqdn:     util.Fqdn(hostname),
		EC2Hostname:    "", // TODO
	}
}

func buildKey(key string) string {
	return path.Join(metadata.CachePrefix, packageCachePrefix, key)
}
