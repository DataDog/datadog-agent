// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Portions of this code are taken from the gopsutil project
// https://github.com/shirou/gopsutil .  This code is licensed under the New BSD License
// copyright WAKAYAMA Shirou, and the gopsutil contributors

// Package utils generate host metadata payloads ready to be sent.
package utils

import (
	"context"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/hosttags"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp"
	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	containerMetadata "github.com/DataDog/datadog-agent/pkg/util/containers/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	hostCacheKey        = cache.BuildAgentKey("host", "utils", "host")
	systemStatsCacheKey = cache.BuildAgentKey("host", "utils", "systemStats")
	hostInfoCacheKey    = cache.BuildAgentKey("host", "utils", "hostInfo")

	// for testing
	otlpIsEnabled  = otlp.IsEnabled
	installinfoGet = installinfo.Get
)

type systemStats struct {
	CPUCores  int32     `json:"cpuCores"`
	Machine   string    `json:"machine"`
	Platform  string    `json:"platform"`
	Pythonv   string    `json:"pythonV"`
	Processor string    `json:"processor"`
	Macver    osVersion `json:"macV"`
	Nixver    osVersion `json:"nixV"`
	Fbsdver   osVersion `json:"fbsdV"`
	Winver    osVersion `json:"winV"`
}

// LogsMeta is metadata about the host's logs agent
type LogsMeta struct {
	Transport            string `json:"transport"`
	AutoMultilineEnabled bool   `json:"auto_multi_line_detection_enabled"`
}

// NetworkMeta is metadata about the host's network
type NetworkMeta struct {
	ID         string `json:"network-id"`
	PublicIPv4 string `json:"public-ipv4,omitempty"`
}

// InstallMethod is metadata about the agent's installation
type InstallMethod struct {
	Tool             *string `json:"tool"`
	ToolVersion      string  `json:"tool_version"`
	InstallerVersion *string `json:"installer_version"`
}

// ProxyMeta is metatdata about the proxy configuration
type ProxyMeta struct {
	NoProxyNonexactMatch              bool `json:"no-proxy-nonexact-match"`
	ProxyBehaviorChanged              bool `json:"proxy-behavior-changed"`
	NoProxyNonexactMatchExplicitlySet bool `json:"no-proxy-nonexact-match-explicitly-set"`
}

// OtlpMeta is metadata about the otlp pipeline
type OtlpMeta struct {
	Enabled bool `json:"enabled"`
}

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Os            string            `json:"os"`
	AgentFlavor   string            `json:"agent-flavor"`
	PythonVersion string            `json:"python"`
	SystemStats   *systemStats      `json:"systemStats"`
	Meta          *Meta             `json:"meta"`
	HostTags      *hosttags.Tags    `json:"host-tags"`
	ContainerMeta map[string]string `json:"container-meta,omitempty"`
	NetworkMeta   *NetworkMeta      `json:"network"`
	LogsMeta      *LogsMeta         `json:"logs"`
	InstallMethod *InstallMethod    `json:"install-method"`
	ProxyMeta     *ProxyMeta        `json:"proxy-info"`
	OtlpMeta      *OtlpMeta         `json:"otlp"`
}

func getNetworkMeta(ctx context.Context) *NetworkMeta {
	nid, err := cloudproviders.GetNetworkID(ctx)
	if err != nil {
		log.Infof("could not get network metadata: %s", err)
		return nil
	}

	networkMeta := &NetworkMeta{
		ID: nid,
	}

	publicIPv4, err := cloudproviders.GetPublicIPv4(ctx)
	if err == nil {
		log.Infof("Adding public IPv4 %s to network metadata", publicIPv4)
		networkMeta.PublicIPv4 = publicIPv4
	}
	return networkMeta
}

func getLogsMeta(conf model.Reader) *LogsMeta {
	return &LogsMeta{
		Transport:            string(status.GetCurrentTransport()),
		AutoMultilineEnabled: conf.GetBool("logs_config.auto_multi_line_detection"),
	}
}

func getInstallMethod(conf model.Reader) *InstallMethod {
	install, err := installinfoGet(conf)
	if err != nil {
		return &InstallMethod{
			ToolVersion:      "undefined",
			Tool:             nil,
			InstallerVersion: nil,
		}
	}
	return &InstallMethod{
		ToolVersion:      install.ToolVersion,
		Tool:             &install.Tool,
		InstallerVersion: &install.InstallerVersion,
	}
}

// getProxyMeta exposes the value of no_proxy_nonexact_match as well as any warnings of proxy behavior change in the
// metadata payload. The NoProxy maps contain any errors or warnings due to the behavior changing when
// no_proxy_nonexact_match is enabled. ProxyBehaviorChanged is true in the metadata if there would be any errors or
// warnings indicating that there would a behavior change if 'no_proxy_nonexact_match' was enabled.
func getProxyMeta(conf model.Reader) *ProxyMeta {
	NoProxyNonexactMatchExplicitlySetState := false
	NoProxyNonexactMatch := false
	if conf.IsSet("no_proxy_nonexact_match") {
		NoProxyNonexactMatchExplicitlySetState = true
		NoProxyNonexactMatch = conf.GetBool("no_proxy_nonexact_match")
	}

	return &ProxyMeta{
		NoProxyNonexactMatch:              NoProxyNonexactMatch,
		ProxyBehaviorChanged:              httputils.GetNumberOfWarnings() > 0,
		NoProxyNonexactMatchExplicitlySet: NoProxyNonexactMatchExplicitlySetState,
	}
}

// GetOSVersion returns the current OS version
func GetOSVersion() string {
	hostInfo := GetInformation()
	return strings.Trim(hostInfo.Platform+" "+hostInfo.PlatformVersion, " ")
}

// GetPayload builds a metadata payload every time is called.
// Some data is collected only once, some is cached, some is collected at every call.
func GetPayload(ctx context.Context, conf model.Reader) *Payload {
	hostnameData, err := hostname.GetWithProvider(ctx)
	if err != nil {
		log.Errorf("Error grabbing hostname for status: %v", err)
		hostnameData = hostname.Data{Hostname: "unknown", Provider: "unknown"}
	}

	meta := getMeta(ctx, conf)
	meta.Hostname = hostnameData.Hostname

	p := &Payload{
		Os:            osName,
		AgentFlavor:   flavor.GetFlavor(),
		PythonVersion: python.GetPythonInfo(),
		SystemStats:   getSystemStats(),
		Meta:          meta,
		HostTags:      hosttags.Get(ctx, false, conf),
		ContainerMeta: containerMetadata.Get(1 * time.Second),
		NetworkMeta:   getNetworkMeta(ctx),
		LogsMeta:      getLogsMeta(conf),
		InstallMethod: getInstallMethod(conf),
		ProxyMeta:     getProxyMeta(conf),
		OtlpMeta:      &OtlpMeta{Enabled: otlpIsEnabled(conf)},
	}

	// Cache the metadata for use in other payloads
	cache.Cache.Set(hostCacheKey, p, cache.NoExpiration)
	return p
}

// GetFromCache returns the payload from the cache if it exists, otherwise it creates it.
// The metadata reporting should always grab it fresh. Any other uses, e.g. status, should use this
func GetFromCache(ctx context.Context, conf model.Reader) *Payload {
	data, found := cache.Cache.Get(hostCacheKey)
	if !found {
		return GetPayload(ctx, conf)
	}
	return data.(*Payload)
}

// GetPlatformName returns the name of the current platform
func GetPlatformName() string {
	return GetInformation().Platform
}
