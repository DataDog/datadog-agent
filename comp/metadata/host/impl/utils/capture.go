// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package utils

import (
	"strings"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

var captureContainerKeys = [...]string{"cri_name", "cri_version", "docker_version", "docker_swarm", "kubelet_version"}

// CaptureMetadataSize conservatively charges the fields CopyCaptureMetadata
// allocates. APIKey and arbitrary configuration/container fields are never read
// by either helper, including when computing the reservation size.
func CaptureMetadataSize(common *CommonPayload, p *Payload) int64 {
	n := int64(8192 + len(common.AgentVersion) + len(common.UUID) + len(common.InternalHostname) + len(p.Os) + len(p.AgentFlavor) + len(p.PythonVersion))
	if stats := p.SystemStats; stats != nil {
		n += int64(len(stats.Machine) + len(stats.Platform) + len(stats.Pythonv) + len(stats.Processor))
		for _, tuple := range []osVersion{stats.Macver, stats.Winver, stats.Nixver, stats.Fbsdver} {
			for _, value := range tuple {
				switch value := any(value).(type) {
				case string:
					n += int64(len(value))
				case [3]string:
					for _, part := range value {
						n += int64(len(part))
					}
				}
			}
		}
	}
	if network := p.NetworkMeta; network != nil {
		n += int64(len(network.ID) + len(network.PublicIPv4))
	}
	if tags := p.HostTags; tags != nil {
		for _, values := range [][]string{tags.System, tags.GoogleCloudPlatform} {
			n += 256 + int64(len(values))*int64(unsafe.Sizeof(""))
			for _, tag := range values {
				n += int64(len(tag))
			}
		}
	}
	// These conversions borrow named scalar structures without allocating or
	// inspecting credentials. Their strings and slices are copied only later.
	borrowed := &telemetrycapture.HostMetadata{Meta: (*telemetrycapture.HostIdentityMetadata)(p.Meta), InstallMethod: (*telemetrycapture.HostInstallMethod)(p.InstallMethod), Logs: (*telemetrycapture.HostLogsMetadata)(p.LogsMeta)}
	n += telemetrycapture.PayloadSize(telemetrycapture.Payload{Metadata: borrowed})
	for _, key := range captureContainerKeys {
		n += 256 + int64(len(key)+len(p.ContainerMeta[key]))
	}

	return n
}

// CopyCaptureMetadata constructs a distinct projection from safe fields. It
// never marshals the original, which contains a legacy API key in its body.
func CopyCaptureMetadata(common *CommonPayload, p *Payload) *telemetrycapture.HostMetadata {
	result := &telemetrycapture.HostMetadata{
		AgentVersion: strings.Clone(common.AgentVersion), UUID: strings.Clone(common.UUID),
		Hostname: strings.Clone(common.InternalHostname), OS: strings.Clone(p.Os), AgentFlavor: strings.Clone(p.AgentFlavor),
		PythonVersion: strings.Clone(p.PythonVersion), FIPSMode: p.FipsMode, FIPSProxyEnabled: p.FipsProxyEnabled,
	}
	if stats := p.SystemStats; stats != nil {
		result.CPUCores, result.Machine, result.Platform = int(stats.CPUCores), strings.Clone(stats.Machine), strings.Clone(stats.Platform)
		result.PythonRuntimeVersion, result.Processor = strings.Clone(stats.Pythonv), strings.Clone(stats.Processor)
		result.UnixVersion, result.FreeBSDVersion = copyVersionStrings(stats.Nixver), copyVersionStrings(stats.Fbsdver)
		// The legacy tuple differs between platforms; consume only the fields
		// used by native host metadata, including the release-info tuple.
		for i, value := range stats.Macver {
			if tuple, ok := any(value).([3]string); ok && i == 1 {
				result.MacReleaseInfo = copyStrings(tuple[:])
			}
			if v, ok := any(value).(string); ok {
				switch i {
				case 0:
					result.MacVersion = strings.Clone(v)
				case 2:
					result.MacMachine = strings.Clone(v)
				}
			}
		}
		if p.Os == "win32" || p.Os == "windows" {
			result.Windows = copyVersionStrings(stats.Winver)
		}
	}
	if network := p.NetworkMeta; network != nil {
		result.NetworkID, result.PublicIPv4 = strings.Clone(network.ID), strings.Clone(network.PublicIPv4)
	}
	if tags := p.HostTags; tags != nil {
		result.HostTags = map[string][]string{"system": copyStrings(tags.System)}
		if tags.GoogleCloudPlatform != nil {
			result.HostTags["google cloud platform"] = copyStrings(tags.GoogleCloudPlatform)
		}
	}
	if p.Meta != nil {
		m := (*telemetrycapture.HostIdentityMetadata)(p.Meta)
		result.Meta = &telemetrycapture.HostIdentityMetadata{
			SocketHostname: strings.Clone(m.SocketHostname), SocketFqdn: strings.Clone(m.SocketFqdn), EC2Hostname: strings.Clone(m.EC2Hostname), Hostname: strings.Clone(m.Hostname),
			HostAliases: copyStrings(m.HostAliases), Timezones: copyStrings(m.Timezones), InstanceID: strings.Clone(m.InstanceID), AgentHostname: strings.Clone(m.AgentHostname),
			ClusterName: strings.Clone(m.ClusterName), LegacyResolutionHostname: strings.Clone(m.LegacyResolutionHostname), HostnameResolutionVersion: m.HostnameResolutionVersion, CanonicalCloudResourceID: strings.Clone(m.CanonicalCloudResourceID),
		}
	}
	if p.InstallMethod != nil {
		result.InstallMethod = &telemetrycapture.HostInstallMethod{Tool: copyStringPointer(p.InstallMethod.Tool), ToolVersion: strings.Clone(p.InstallMethod.ToolVersion), InstallerVersion: copyStringPointer(p.InstallMethod.InstallerVersion)}
	}
	if p.LogsMeta != nil {
		result.Logs = &telemetrycapture.HostLogsMetadata{Transport: strings.Clone(p.LogsMeta.Transport), AutoMultilineEnabled: p.LogsMeta.AutoMultilineEnabled}
	}
	if p.ProxyMeta != nil {
		proxy := telemetrycapture.HostProxyMetadata(*p.ProxyMeta)
		result.Proxy = &proxy
	}
	for _, key := range captureContainerKeys {
		if value, ok := p.ContainerMeta[key]; ok {
			if result.ContainerMeta == nil {
				result.ContainerMeta = make(map[string]string)
			}
			result.ContainerMeta[strings.Clone(key)] = strings.Clone(value)
		}
	}
	if p.OtlpMeta != nil {
		result.OTLPEnabled = p.OtlpMeta.Enabled
	}

	return result
}

func copyStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	owned := strings.Clone(*value)
	return &owned
}

func copyStrings(values []string) []string {
	if values == nil {
		return nil
	}
	owned := make([]string, len(values))
	for i, value := range values {
		owned[i] = strings.Clone(value)
	}
	return owned
}

func copyVersionStrings(version osVersion) []string {
	owned := make([]string, len(version))
	for i, value := range version {
		if text, ok := any(value).(string); ok {
			owned[i] = strings.Clone(text)
		}
	}
	return owned
}
