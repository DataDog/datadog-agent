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
	"testing"
)

func TestCaptureMetadataProjectionOmitsCredentialsAndOwnsFields(t *testing.T) {
	common := CommonPayload{APIKey: "credential-sentinel", AgentVersion: "7.producer", UUID: "native-id", InternalHostname: "native-host"}
	p := Payload{
		Os: "windows", AgentFlavor: "agent", PythonVersion: "unrelated-python-sentinel",
		SystemStats:   &systemStats{CPUCores: 8, Machine: "arm64", Platform: "platform", Pythonv: "unrelated-python-sentinel"},
		HostTags:      &hosttags.Tags{System: []string{"native:tag"}, GoogleCloudPlatform: []string{"cloud-sentinel"}},
		NetworkMeta:   &NetworkMeta{ID: "network-id", PublicIPv4: "public-ip-sentinel"},
		ContainerMeta: map[string]string{"secret": "container-sentinel"},
	}
	p.SystemStats.Winver[0] = "10"
	p.SystemStats.Winver[1] = "build"
	projection := CopyCaptureMetadata(&common, &p)
	require.LessOrEqual(t, telemetrycapture.PayloadSize(telemetrycapture.Payload{Metadata: projection}), CaptureMetadataSize(&common, &p))
	require.Equal(t, "7.producer", projection.AgentVersion)
	require.Equal(t, 8, projection.CPUCores)
	require.Equal(t, []string{"10", "build"}, projection.Windows)
	encoded, err := json.Marshal(projection)
	require.NoError(t, err)
	for _, sentinel := range []string{"credential-sentinel", "cloud-sentinel", "container-sentinel", "public-ip-sentinel", "unrelated-python-sentinel"} {
		require.NotContains(t, string(encoded), sentinel)
	}
	p.HostTags.System[0] = "mutated"
	p.NetworkMeta.ID = "mutated"
	p.SystemStats.Winver[0] = "mutated"
	common.AgentVersion = "mutated"
	require.Equal(t, []string{"native:tag"}, projection.HostTags["system"])
	require.Equal(t, "network-id", projection.NetworkID)
	require.Equal(t, []string{"10", "build"}, projection.Windows)
	require.Equal(t, "7.producer", projection.AgentVersion)
	require.Equal(t, "credential-sentinel", common.APIKey)
}
