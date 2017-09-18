// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package host

import (
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/common"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/host"

	"github.com/DataDog/datadog-agent/pkg/util/azure"
	"github.com/DataDog/datadog-agent/pkg/util/cloudfoundry"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/gce"
	log "github.com/cihub/seelog"
)

const packageCachePrefix = "host"

// GetPayload builds a metadata payload every time is called.
// Some data is collected only once, some is cached, some is collected at every call.
func GetPayload(hostname string) *Payload {
	meta := getMeta()
	meta.Hostname = hostname

	return &Payload{
		Os:            runtime.GOOS,
		PythonVersion: getPythonVersion(),
		SystemStats:   getSystemStats(),
		Meta:          meta,
		HostTags:      getHostTags(),
	}
}

// GetStatusInformation just returns an InfoStat object, we need some additional information that's not
func GetStatusInformation() *host.InfoStat {
	return getHostInfo()
}

// GetMeta grabs the metadata from the cache and returns it,
// if the cache is empty, then it queries the information directly
func GetMeta() *Meta {
	key := buildKey("meta")
	if x, found := cache.Cache.Get(key); found {
		return x.(*Meta)
	}
	return getMeta()
}

func getHostTags() *tags {
	hostTags := config.Datadog.GetStringSlice("tags")
	var gceTags []string

	ec2Tags, err := ec2.GetTags()
	if err != nil {
		log.Warnf("No EC2 host tags %v", err)
	}
	hostTags = append(hostTags, ec2Tags...)

	gceTags, err = gce.GetTags()
	if err != nil {
		log.Warnf("No GCE host tags %v", err)
	}

	return &tags{
		System:              hostTags,
		GoogleCloudPlatform: gceTags,
	}
}

func getSystemStats() *systemStats {
	var stats *systemStats
	key := buildKey("systemStats")
	if x, found := cache.Cache.Get(key); found {
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
		cache.Cache.Set(key, stats, cache.NoExpiration)
	}

	return stats
}

// getPythonVersion returns the version string as provided by the embedded Python
// interpreter. The string is stored in the Agent cache when the interpreter is
// initialized (see pkg/collector/py/utils.go), an empty value is expected when
// using this package without embedding Python.
func getPythonVersion() string {
	// retrieve the Python version from the Agent cache
	if x, found := cache.Cache.Get(cache.BuildAgentKey("pythonVersion")); found {
		return x.(string)
	}

	return "n/a"
}

// getCPUInfo returns InfoStat for the first CPU gopsutil found
func getCPUInfo() *cpu.InfoStat {
	key := buildKey("cpuInfo")
	if x, found := cache.Cache.Get(key); found {
		return x.(*cpu.InfoStat)
	}

	i, err := cpu.Info()
	if err != nil {
		// don't cache and return zero value
		log.Errorf("failed to retrieve cpu info: %s", err)
		return &cpu.InfoStat{}
	}
	info := &i[0]
	cache.Cache.Set(key, info, cache.NoExpiration)
	return info
}

func getHostInfo() *host.InfoStat {
	key := buildKey("hostInfo")
	if x, found := cache.Cache.Get(key); found {
		return x.(*host.InfoStat)
	}

	info, err := host.Info()
	if err != nil {
		// don't cache and return zero value
		log.Errorf("failed to retrieve host info: %s", err)
		return &host.InfoStat{}
	}
	cache.Cache.Set(key, info, cache.NoExpiration)
	return info
}

// getHostAliases returns the hostname aliases from different provider
// This should include GCE, Azure, Cloud foundry.
func getHostAliases() []string {
	aliases := []string{}

	azureAlias, err := azure.GetHostAlias()
	if err != nil {
		log.Errorf("no Azure Host Alias: %s", err)
	} else if azureAlias != "" {
		aliases = append(aliases, azureAlias)
	}

	gceAlias, err := gce.GetHostAlias()
	if err != nil {
		log.Errorf("no GCE Host Alias: %s", err)
	} else {
		aliases = append(aliases, gceAlias)
	}

	cfAlias, err := cloudfoudry.GetHostAlias()
	if err != nil {
		log.Errorf("no Cloud Foundry Host Alias: %s", err)
	} else if cfAlias != "" {
		aliases = append(aliases, cfAlias)
	}
	return aliases
}

// getMeta grabs the information and refreshes the cache
func getMeta() *Meta {
	hostname, _ := os.Hostname()
	tzname, _ := time.Now().Zone()
	ec2Hostname, _ := ec2.GetHostname()

	m := &Meta{
		SocketHostname: hostname,
		Timezones:      []string{tzname},
		SocketFqdn:     util.Fqdn(hostname),
		EC2Hostname:    ec2Hostname,
		HostAliases:    getHostAliases(),
	}

	// Cache the metadata for use in other payload
	key := buildKey("meta")
	cache.Cache.Set(key, m, cache.NoExpiration)

	return m
}

func buildKey(key string) string {
	return path.Join(common.CachePrefix, packageCachePrefix, key)
}
