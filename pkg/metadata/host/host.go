// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package host

import (
	"os"
	"path"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/common"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/alibaba"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/tencent"

	"github.com/DataDog/datadog-agent/pkg/metadata/host/container"
	"github.com/DataDog/datadog-agent/pkg/util/azure"
	"github.com/DataDog/datadog-agent/pkg/util/cloudfoundry"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/gce"
	kubelet "github.com/DataDog/datadog-agent/pkg/util/hostname/kubelet"

	"github.com/DataDog/datadog-agent/pkg/logs"

	"io/ioutil"

	yaml "gopkg.in/yaml.v2"
)

const packageCachePrefix = "host"

type installInfo struct {
	Method struct {
		Tool             string `yaml:"tool"`
		ToolVersion      string `yaml:"tool_version"`
		InstallerVersion string `yaml:"installer_version"`
	} `yaml:"install_method"`
}

// GetPayload builds a metadata payload every time is called.
// Some data is collected only once, some is cached, some is collected at every call.
func GetPayload(hostnameData util.HostnameData) *Payload {
	meta := getMeta(hostnameData)
	meta.Hostname = hostnameData.Hostname

	p := &Payload{
		Os:            osName,
		AgentFlavor:   config.AgentFlavor,
		PythonVersion: GetPythonVersion(),
		SystemStats:   getSystemStats(),
		Meta:          meta,
		HostTags:      getHostTags(),
		ContainerMeta: getContainerMeta(1 * time.Second),
		NetworkMeta:   getNetworkMeta(),
		LogsMeta:      getLogsMeta(),
		InstallMethod: getInstallMethod(getInstallInfoPath()),
	}

	// Cache the metadata for use in other payloads
	key := buildKey("payload")
	cache.Cache.Set(key, p, cache.NoExpiration)

	return p
}

// GetPayloadFromCache returns the payload from the cache if it exists, otherwise it creates it.
// The metadata reporting should always grab it fresh. Any other uses, e.g. status, should use this
func GetPayloadFromCache(hostnameData util.HostnameData) *Payload {
	key := buildKey("payload")
	if x, found := cache.Cache.Get(key); found {
		return x.(*Payload)
	}
	return GetPayload(hostnameData)
}

// GetMeta grabs the metadata from the cache and returns it,
// if the cache is empty, then it queries the information directly
func GetMeta(hostnameData util.HostnameData) *Meta {
	key := buildKey("meta")
	if x, found := cache.Cache.Get(key); found {
		return x.(*Meta)
	}
	return getMeta(hostnameData)
}

// GetPythonVersion returns the version string as provided by the embedded Python
// interpreter.
func GetPythonVersion() string {
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

	alibabaAlias, err := alibaba.GetHostAlias()
	if err != nil {
		log.Debugf("no Alibaba Host Alias: %s", err)
	} else if alibabaAlias != "" {
		aliases = append(aliases, alibabaAlias)
	}

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

	cfAliases, err := cloudfoundry.GetHostAliases()
	if err != nil {
		log.Debugf("no Cloud Foundry Host Alias: %s", err)
	} else if cfAliases != nil {
		aliases = append(aliases, cfAliases...)
	}

	k8sAlias, err := kubelet.GetHostAlias()
	if err != nil {
		log.Debugf("no Kubernetes Host Alias (through kubelet API): %s", err)
	} else if k8sAlias != "" {
		aliases = append(aliases, k8sAlias)
	}

	tencentAlias, err := tencent.GetHostAlias()
	if err != nil {
		log.Debugf("no Tencent Host Alias: %s", err)
	} else if tencentAlias != "" {
		aliases = append(aliases, tencentAlias)
	}

	return aliases
}

// getMeta grabs the information and refreshes the cache
func getMeta(hostnameData util.HostnameData) *Meta {
	hostname, _ := os.Hostname()
	tzname, _ := time.Now().Zone()
	ec2Hostname, _ := ec2.GetHostname()
	instanceID, _ := ec2.GetInstanceID()

	var agentHostname string

	if config.Datadog.GetBool("hostname_force_config_as_canonical") &&
		hostnameData.Provider == util.HostnameProviderConfiguration {
		agentHostname = hostnameData.Hostname
	}

	m := &Meta{
		SocketHostname: hostname,
		Timezones:      []string{tzname},
		SocketFqdn:     util.Fqdn(hostname),
		EC2Hostname:    ec2Hostname,
		HostAliases:    getHostAliases(),
		InstanceID:     instanceID,
		AgentHostname:  agentHostname,
	}

	// Cache the metadata for use in other payload
	key := buildKey("meta")
	cache.Cache.Set(key, m, cache.NoExpiration)

	return m
}

func getNetworkMeta() *NetworkMeta {
	nid, err := util.GetNetworkID()
	if err != nil {
		log.Infof("could not get network metadata: %s", err)
		return nil
	}
	return &NetworkMeta{ID: nid}
}

func getContainerMeta(timeout time.Duration) map[string]string {
	wg := sync.WaitGroup{}
	containerMeta := make(map[string]string)
	// protecting the above map from concurrent access
	mutex := &sync.Mutex{}

	for provider, getMeta := range container.DefaultCatalog {
		wg.Add(1)
		go func(provider string, getMeta container.MetadataProvider) {
			defer wg.Done()
			meta, err := getMeta()
			if err != nil {
				log.Debugf("Unable to get %s metadata: %s", provider, err)
				return
			}
			mutex.Lock()
			for k, v := range meta {
				containerMeta[k] = v
			}
			mutex.Unlock()
		}(provider, getMeta)
	}
	// we want to timeout even if the wait group is not done yet
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		return containerMeta
	case <-time.After(timeout):
		// in this case the map might be incomplete so return a copy to avoid race
		incompleteMeta := make(map[string]string)
		mutex.Lock()
		for k, v := range containerMeta {
			incompleteMeta[k] = v
		}
		mutex.Unlock()
		return incompleteMeta
	}
}

func getLogsMeta() *LogsMeta {
	return &LogsMeta{Transport: string(logs.CurrentTransport)}
}

func buildKey(key string) string {
	return path.Join(common.CachePrefix, packageCachePrefix, key)
}

func getInstallInfoPath() string {
	return path.Join(config.FileUsedDir(), "install_info")
}

func getInstallInfo(infoPath string) (*installInfo, error) {
	yamlContent, err := ioutil.ReadFile(infoPath)

	if err != nil {
		return nil, err
	}

	var install installInfo

	if err := yaml.UnmarshalStrict(yamlContent, &install); err != nil {
		// file was manipulated and is not relevant to format
		return nil, err
	}

	return &install, nil
}

func getInstallMethod(infoPath string) *InstallMethod {
	install, err := getInstallInfo(infoPath)

	// if we could not get install info
	if err != nil {
		// consider install info is kept "undefined"
		return &InstallMethod{
			ToolVersion:      "undefined",
			Tool:             nil,
			InstallerVersion: nil,
		}
	}

	return &InstallMethod{
		ToolVersion:      install.Method.ToolVersion,
		Tool:             &install.Method.Tool,
		InstallerVersion: &install.Method.InstallerVersion,
	}
}
