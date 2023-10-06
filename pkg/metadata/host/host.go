// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package host

import (
	"context"
	"os"
	"path"
	"time"

	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/utils"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp"
	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/config"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/metadata/common"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	containerMetadata "github.com/DataDog/datadog-agent/pkg/util/containers/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"

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
	meta := hostMetadataUtils.GetMeta(ctx, config.Datadog)
	meta.Hostname = hostnameData.Hostname

	p := &Payload{
		Os:            osName,
		AgentFlavor:   flavor.GetFlavor(),
		PythonVersion: python.GetPythonInfo(),
		SystemStats:   getSystemStats(),
		Meta:          meta,
		HostTags:      hostMetadataUtils.GetHostTags(ctx, false, config.Datadog),
		ContainerMeta: containerMetadata.Get(1 * time.Second),
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

func getNetworkMeta(ctx context.Context) *NetworkMeta {
	nid, err := cloudproviders.GetNetworkID(ctx)
	if err != nil {
		log.Infof("could not get network metadata: %s", err)
		return nil
	}

	networkMeta := &NetworkMeta{ID: nid}

	publicIPv4, err := cloudproviders.GetPublicIPv4(ctx)
	if err == nil {
		log.Infof("Adding public IPv4 %s to network metadata", publicIPv4)
		networkMeta.PublicIPv4 = publicIPv4
	}

	return networkMeta
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
	NoProxyNonexactMatchExplicitlySetState := false
	NoProxyNonexactMatch := false
	if config.Datadog.IsSet("no_proxy_nonexact_match") {
		NoProxyNonexactMatchExplicitlySetState = true
		NoProxyNonexactMatch = config.Datadog.GetBool("no_proxy_nonexact_match")
	}

	return &ProxyMeta{
		NoProxyNonexactMatch:              NoProxyNonexactMatch,
		ProxyBehaviorChanged:              httputils.GetNumberOfWarnings() > 0,
		NoProxyNonexactMatchExplicitlySet: NoProxyNonexactMatchExplicitlySetState,
	}
}

func buildKey(key string) string {
	return path.Join(common.CachePrefix, packageCachePrefix, key)
}

func getInstallInfoPath() string {
	return path.Join(configUtils.ConfFileDirectory(config.Datadog), "install_info")
}

func getInstallInfo(infoPath string) (*installInfo, error) {
	yamlContent, err := os.ReadFile(infoPath)

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

func getOtlpMeta() *OtlpMeta {
	return &OtlpMeta{Enabled: otlp.IsEnabled(config.Datadog)}
}
