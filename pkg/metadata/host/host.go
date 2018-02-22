// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package host

import (
	"os"
	"path"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/common"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/util/azure"
	"github.com/DataDog/datadog-agent/pkg/util/cloudfoundry"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/gce"
	k8s "github.com/DataDog/datadog-agent/pkg/util/kubernetes/hostinfo"
)

const packageCachePrefix = "host"

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
