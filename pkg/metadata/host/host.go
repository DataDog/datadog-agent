// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package host

import (
	"context"
	"errors"
	"os"
	"path"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/metadata/common"
	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"
	"github.com/DataDog/datadog-agent/pkg/otlp"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/metadata/host/container"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/azure"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/gce"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"

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
func GetPayload(ctx context.Context, hostnameData hostname.Data) *Payload {
	meta := getMeta(ctx, hostnameData)
	meta.Hostname = hostnameData.Hostname

	p := &Payload{
		Os:            osName,
		AgentFlavor:   flavor.GetFlavor(),
		PythonVersion: GetPythonVersion(),
		SystemStats:   getSystemStats(),
		Meta:          meta,
		HostTags:      GetHostTags(ctx, false),
		ContainerMeta: getContainerMeta(1 * time.Second),
		NetworkMeta:   getNetworkMeta(ctx),
		LogsMeta:      getLogsMeta(),
		InstallMethod: getInstallMethod(getInstallInfoPath()),
		ProxyMeta:     getProxyMeta(),
		OtlpMeta:      getOtlpMeta(),
	}

	// Cache the metadata for use in other payloads
	key := buildKey("payload")
	cache.Cache.Set(key, p, cache.NoExpiration)

	return p
}

// GetPayloadFromCache returns the payload from the cache if it exists, otherwise it creates it.
// The metadata reporting should always grab it fresh. Any other uses, e.g. status, should use this
func GetPayloadFromCache(ctx context.Context, hostnameData hostname.Data) *Payload {
	key := buildKey("payload")
	if x, found := cache.Cache.Get(key); found {
		return x.(*Payload)
	}
	return GetPayload(ctx, hostnameData)
}

// GetMeta grabs the metadata from the cache and returns it,
// if the cache is empty, then it queries the information directly
func GetMeta(ctx context.Context, hostnameData hostname.Data) *Meta {
	key := buildKey("meta")
	if x, found := cache.Cache.Get(key); found {
		return x.(*Meta)
	}
	return getMeta(ctx, hostnameData)
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

func getPublicIPv4(ctx context.Context) (string, error) {
	publicIPFetcher := map[string]func(context.Context) (string, error){
		"EC2":   ec2.GetPublicIPv4,
		"GCE":   gce.GetPublicIPv4,
		"Azure": azure.GetPublicIPv4,
	}
	for name, fetcher := range publicIPFetcher {
		publicIPv4, err := fetcher(ctx)
		if err == nil {
			log.Debugf("%s public IP = %s", name, publicIPv4)
			return publicIPv4, nil
		}
		log.Debugf("could not fetch %s public IPv4: %s", name, err)
	}
	log.Infof("No public IPv4 address found")
	return "", errors.New("No public IPv4 address found")
}

// getMeta grabs the information and refreshes the cache
func getMeta(ctx context.Context, hostnameData hostname.Data) *Meta {
	osHostname, _ := os.Hostname()
	tzname, _ := time.Now().Zone()
	ec2Hostname, _ := ec2.GetHostname(ctx)
	instanceID, _ := ec2.GetInstanceID(ctx)

	var agentHostname string

	if config.Datadog.GetBool("hostname_force_config_as_canonical") && hostnameData.FromConfiguration() {
		agentHostname = hostnameData.Hostname
	}

	m := &Meta{
		SocketHostname: osHostname,
		Timezones:      []string{tzname},
		SocketFqdn:     util.Fqdn(osHostname),
		EC2Hostname:    ec2Hostname,
		HostAliases:    cloudproviders.GetHostAliases(ctx),
		InstanceID:     instanceID,
		AgentHostname:  agentHostname,
	}

	if finalClusterName := kubelet.GetMetaClusterNameText(ctx, osHostname); finalClusterName != "" {
		m.ClusterName = finalClusterName
	}

	// Cache the metadata for use in other payload
	key := buildKey("meta")
	cache.Cache.Set(key, m, cache.NoExpiration)

	return m
}

func getNetworkMeta(ctx context.Context) *NetworkMeta {
	nid, err := cloudproviders.GetNetworkID(ctx)
	if err != nil {
		log.Infof("could not get network metadata: %s", err)
		return nil
	}

	networkMeta := &NetworkMeta{ID: nid}

	publicIPv4, err := getPublicIPv4(ctx)
	if err == nil {
		log.Infof("Adding public IPv4 %s to network metadata", publicIPv4)
		networkMeta.PublicIPv4 = publicIPv4
	}

	return networkMeta
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
	return &LogsMeta{
		Transport:            string(status.CurrentTransport),
		AutoMultilineEnabled: config.Datadog.GetBool("logs_config.auto_multi_line_detection"),
	}
}

// Expose the value of no_proxy_nonexact_match as well as any warnings of proxy behavior change in the metadata payload.
// The NoProxy maps contain any errors or warnings due to the behavior changing when no_proxy_nonexact_match is enabled.
// ProxyBehaviorChanged is true in the metadata if there would be any errors or warnings indicating that there would a
// behavior change if 'no_proxy_nonexact_match' was enabled.
func getProxyMeta() *ProxyMeta {
	httputils.NoProxyMapMutex.Lock()
	defer httputils.NoProxyMapMutex.Unlock()

	return &ProxyMeta{
		NoProxyNonexactMatch: config.Datadog.GetBool("no_proxy_nonexact_match"),
		ProxyBehaviorChanged: len(httputils.NoProxyIgnoredWarningMap)+len(httputils.NoProxyUsedInFuture)+len(httputils.NoProxyChanged) > 0,
	}
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
		inventories.SetAgentMetadata(inventories.AgentInstallMethodTool, "undefined")
		inventories.SetAgentMetadata(inventories.AgentInstallMethodToolVersion, "")
		inventories.SetAgentMetadata(inventories.AgentInstallMethodInstallerVersion, "")
		// consider install info is kept "undefined"
		return &InstallMethod{
			ToolVersion:      "undefined",
			Tool:             nil,
			InstallerVersion: nil,
		}
	}

	inventories.SetAgentMetadata(inventories.AgentInstallMethodTool, install.Method.Tool)
	inventories.SetAgentMetadata(inventories.AgentInstallMethodToolVersion, install.Method.ToolVersion)
	inventories.SetAgentMetadata(inventories.AgentInstallMethodInstallerVersion, install.Method.InstallerVersion)
	return &InstallMethod{
		ToolVersion:      install.Method.ToolVersion,
		Tool:             &install.Method.Tool,
		InstallerVersion: &install.Method.InstallerVersion,
	}
}

func getOtlpMeta() *OtlpMeta {
	return &OtlpMeta{Enabled: otlp.IsEnabled(config.Datadog)}
}
