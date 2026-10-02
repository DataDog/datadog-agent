// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package utils

import (
	"encoding/json"
	"github.com/DataDog/datadog-agent/comp/metadata/host/impl/hosttags"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestCaptureMetadataProjectionOmitsCredentialsAndOwnsFields(t *testing.T) {
	tool, installer := "installer", "1.2.3"
	common := CommonPayload{APIKey: "credential-sentinel", AgentVersion: "7.producer", UUID: "native-id", InternalHostname: "native-host"}
	p := Payload{
		Os: "windows", AgentFlavor: "agent", PythonVersion: "unrelated-python-sentinel",
		SystemStats:   &systemStats{CPUCores: 8, Machine: "arm64", Platform: "platform", Pythonv: "3.12", Processor: "Apple M4"},
		Meta:          &Meta{SocketHostname: "native-host", SocketFqdn: "native-host.example.org", Timezones: []string{"EDT"}, HostAliases: []string{"native-alias"}},
		InstallMethod: &InstallMethod{Tool: &tool, ToolVersion: "2", InstallerVersion: &installer},
		LogsMeta:      &LogsMeta{Transport: "HTTP", AutoMultilineEnabled: true},
		ProxyMeta:     &ProxyMeta{ProxyBehaviorChanged: true},
		OtlpMeta:      &OtlpMeta{Enabled: true}, FipsMode: true, FipsProxyEnabled: true,
		HostTags:      &hosttags.Tags{System: []string{"native:tag"}, GoogleCloudPlatform: []string{"cloud-sentinel"}},
		NetworkMeta:   &NetworkMeta{ID: "network-id", PublicIPv4: "public-ip-sentinel"},
		ContainerMeta: map[string]string{"secret": "container-sentinel", "docker_version": "24", "cri_name": "containerd"},
	}
	p.SystemStats.Winver[0] = "10"
	p.SystemStats.Winver[1] = "build"
	projection := CopyCaptureMetadata(&common, &p)
	require.LessOrEqual(t, telemetrycapture.PayloadSize(telemetrycapture.Payload{Metadata: projection}), CaptureMetadataSize(&common, &p))
	require.Equal(t, "7.producer", projection.AgentVersion)
	require.Equal(t, 8, projection.CPUCores)
	require.Equal(t, []string{"10", "build"}, projection.Windows[:2])
	require.Len(t, projection.Windows, len(p.SystemStats.Winver))
	encoded, err := json.Marshal(projection)
	require.NoError(t, err)
	for _, sentinel := range []string{"credential-sentinel", "container-sentinel"} {
		require.NotContains(t, string(encoded), sentinel)
	}
	require.Equal(t, p.PythonVersion, projection.PythonVersion)
	require.Equal(t, "3.12", projection.PythonRuntimeVersion)
	require.Equal(t, "Apple M4", projection.Processor)
	require.Equal(t, "public-ip-sentinel", projection.PublicIPv4)
	require.Equal(t, []string{"cloud-sentinel"}, projection.HostTags["google cloud platform"])
	require.Equal(t, "native-host.example.org", projection.Meta.SocketFqdn)
	require.Equal(t, tool, *projection.InstallMethod.Tool)
	require.Equal(t, installer, *projection.InstallMethod.InstallerVersion)
	require.Equal(t, "HTTP", projection.Logs.Transport)
	require.Equal(t, map[string]string{"docker_version": "24", "cri_name": "containerd"}, projection.ContainerMeta)
	require.True(t, projection.Proxy.ProxyBehaviorChanged)
	require.True(t, projection.OTLPEnabled && projection.FIPSMode && projection.FIPSProxyEnabled)
	tool, installer = "changed", "changed"
	p.Meta.HostAliases[0], p.Meta.Timezones[0], p.HostTags.GoogleCloudPlatform[0] = "changed", "changed", "changed"
	require.Equal(t, "installer", *projection.InstallMethod.Tool)
	require.Equal(t, "1.2.3", *projection.InstallMethod.InstallerVersion)
	require.Equal(t, []string{"native-alias"}, projection.Meta.HostAliases)
	require.Equal(t, []string{"EDT"}, projection.Meta.Timezones)
	require.Equal(t, []string{"cloud-sentinel"}, projection.HostTags["google cloud platform"])
	p.HostTags.System[0] = "mutated"
	p.NetworkMeta.ID = "mutated"
	p.SystemStats.Winver[0] = "mutated"
	common.AgentVersion = "mutated"
	require.Equal(t, []string{"native:tag"}, projection.HostTags["system"])
	require.Equal(t, "network-id", projection.NetworkID)
	require.Equal(t, []string{"10", "build"}, projection.Windows[:2])
	require.Len(t, projection.Windows, len(p.SystemStats.Winver))
	require.Equal(t, "7.producer", projection.AgentVersion)
	require.Equal(t, "credential-sentinel", common.APIKey)
}

func TestCaptureMetadataReservesLargeIdentityAndInstallationFields(t *testing.T) {
	text := strings.Repeat("native", 10000)
	p := Payload{Meta: &Meta{SocketFqdn: text, HostAliases: []string{text}, Timezones: []string{text}}, InstallMethod: &InstallMethod{Tool: &text, InstallerVersion: &text, ToolVersion: text}, LogsMeta: &LogsMeta{Transport: text}}
	common := CommonPayload{}
	owned := CopyCaptureMetadata(&common, &p)
	require.GreaterOrEqual(t, CaptureMetadataSize(&common, &p), telemetrycapture.PayloadSize(telemetrycapture.Payload{Metadata: owned}))
	require.Zero(t, testing.AllocsPerRun(100, func() { CaptureMetadataSize(&common, &p) }))
}
