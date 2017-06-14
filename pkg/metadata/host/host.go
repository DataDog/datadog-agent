package host

import (
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metadata/common"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/host"

	"github.com/DataDog/datadog-agent/pkg/util/ec2"
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

// getPythonVersion returns the version string as provided by the embedded Python
// interpreter. The string is stored in the Agent cache when the interpreter is
// initialized (see pkg/collector/py/utils.go), an empty value is expected when
// using this package without embedding Python.
func getPythonVersion() string {
	// retrieve the Python version from the Agent cache
	key := path.Join(util.AgentCachePrefix, "pythonVersion")
	if x, found := util.Cache.Get(key); found {
		return x.(string)
	}

	return "n/a"
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
	ec2Hostname, _ := ec2.GetHostname()

	return &meta{
		SocketHostname: hostname,
		Timezones:      []string{tzname},
		SocketFqdn:     util.Fqdn(hostname),
		EC2Hostname:    ec2Hostname,
	}
}

func buildKey(key string) string {
	return path.Join(common.CachePrefix, packageCachePrefix, key)
}
