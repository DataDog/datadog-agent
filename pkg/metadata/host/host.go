// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package host

import (
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	log "github.com/cihub/seelog"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/host"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/common"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/azure"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/cloudfoundry"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/gce"
	k8s "github.com/DataDog/datadog-agent/pkg/util/kubernetes/hostinfo"
)

const packageCachePrefix = "host"

// Collect at init time
var cpuInfo []cpu.InfoStat

// InitHostMetadata initializes necessary CPU info
func InitHostMetadata() error {
	// Collect before even loading any python check to avoid
	// COM model mayhem on windows
	var err error
	cpuInfo, err = cpu.Info()

	return err
}

// GetPayload builds a metadata payload every time is called.
// Some data is collected only once, some is cached, some is collected at every call.
func GetPayload(hostname string) *Payload {
	meta := getMeta()
	meta.Hostname = hostname

	p := &Payload{
		Os:            osName,
		PythonVersion: getPythonVersion(),
		SystemStats:   getSystemStats(),
		Meta:          meta,
		HostTags:      getHostTags(),
	}

	// Cache the metadata for use in other payloads
	key := buildKey("payload")
	cache.Cache.Set(key, p, cache.NoExpiration)

	return p
}

// GetPayloadFromCache returns the payload from the cache if it exists, otherwise it creates it.
// The metadata reporting should always grab it fresh. Any other uses, e.g. status, should use this
func GetPayloadFromCache(hostname string) *Payload {
	key := buildKey("payload")
	if x, found := cache.Cache.Get(key); found {
		return x.(*Payload)
	}
	return GetPayload(hostname)
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

	if config.Datadog.GetBool("collect_ec2_tags") {
		ec2Tags, err := ec2.GetTags()
		if err != nil {
			log.Debugf("No EC2 host tags %v", err)
		} else {
			hostTags = append(hostTags, ec2Tags...)
		}
	}

	k8sTags, err := k8s.GetTags()
	if err != nil {
		log.Debugf("No Kubernetes host tags %v", err)
	} else {
		hostTags = append(hostTags, k8sTags...)
	}

	gceTags, err := gce.GetTags()
	if err != nil {
		log.Debugf("No GCE host tags %v", err)
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
			Platform:  osName,
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

	if cpuInfo == nil {
		// don't cache and return zero value
		log.Errorf("failed to retrieve cpu info at init time")
		return &cpu.InfoStat{}
	}
	info := &cpuInfo[0]
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
// This should include GCE, Azure, Cloud foundry, kubernetes
func getHostAliases() []string {
	aliases := []string{}

	azureAlias, err := azure.GetHostAlias()
	if err != nil {
		log.Debugf("no Azure Host Alias: %s", err)
	} else if azureAlias != "" {
		aliases = append(aliases, azureAlias)
	}

	gceAlias, err := gce.GetHostAlias()
	if err != nil {
		log.Debugf("no GCE Host Alias: %s", err)
	} else {
		aliases = append(aliases, gceAlias)
	}

	cfAlias, err := cloudfoudry.GetHostAlias()
	if err != nil {
		log.Debugf("no Cloud Foundry Host Alias: %s", err)
	} else if cfAlias != "" {
		aliases = append(aliases, cfAlias)
	}

	k8sAlias, err := k8s.GetHostAlias()
	if err != nil {
		log.Debugf("no Kubernetes Host Alias (through kubelet API): %s", err)
	} else if k8sAlias != "" {
		aliases = append(aliases, k8sAlias)
	}
	return aliases
}

// getMeta grabs the information and refreshes the cache
func getMeta() *Meta {
	hostname, _ := os.Hostname()
	tzname, _ := time.Now().Zone()
	ec2Hostname, _ := ec2.GetHostname()
	instanceID, _ := ec2.GetInstanceID()

	m := &Meta{
		SocketHostname: hostname,
		Timezones:      []string{tzname},
		SocketFqdn:     util.Fqdn(hostname),
		EC2Hostname:    ec2Hostname,
		HostAliases:    getHostAliases(),
		InstanceID:     instanceID,
	}

	// Cache the metadata for use in other payload
	key := buildKey("meta")
	cache.Cache.Set(key, m, cache.NoExpiration)

	return m
}

func buildKey(key string) string {
	return path.Join(common.CachePrefix, packageCachePrefix, key)
}
