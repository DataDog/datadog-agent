// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package inventoryhostimpl

import (
	"encoding/json"
	"testing"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
	"github.com/stretchr/testify/require"
)

func TestCaptureInventoryHostPreservesNativeTelemetryAndOwnsFields(t *testing.T) {
	p := &Payload{Hostname: "native-host", UUID: "native-uuid", Timestamp: 123456789, Metadata: &hostMetadata{
		CPUCores: 4, CPULogicalProcessors: 8, CPUModel: "Example CPU", CPUFrequency: 2200.5, MemoryTotalKb: 12345678,
		OS: "Darwin", OsVersion: "26.0", KernelName: "Darwin", KernelRelease: "25.0.0", CPUArchitecture: "arm64", AgentVersion: "7.85.0",
		IPAddress: "10.0.0.1", Interfaces: "private-sentinel", CloudProviderHostID: "private-sentinel", CloudProviderAccountID: "private-sentinel",
		KernelVersion: "private-sentinel", DmiProductUUID: "private-sentinel", DmiBoardAssetTag: "private-sentinel", HypervisorGuestUUID: "private-sentinel"}}
	before, err := p.MarshalJSON()
	require.NoError(t, err)
	owned := p.CopyCaptureInventory()
	require.GreaterOrEqual(t, p.CaptureInventorySize(), telemetrycapture.PayloadSize(telemetrycapture.Payload{Inventory: owned}))
	require.False(t, unsafe.StringData(p.Metadata.CPUModel) == unsafe.StringData(owned.Host.CPUModel))
	require.Equal(t, p.Metadata.MemoryTotalKb, owned.Host.MemoryTotalKb)
	require.Equal(t, p.Metadata.OsVersion, owned.Host.OSVersion)
	after, err := p.MarshalJSON()
	require.NoError(t, err)
	require.Equal(t, before, after)
	encoded, err := json.Marshal(owned)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(encoded), "every native host inventory field must survive capture")
	require.Equal(t, p.Metadata.Interfaces, owned.Host.Interfaces)
	require.False(t, unsafe.StringData(p.Metadata.Interfaces) == unsafe.StringData(owned.Host.Interfaces))
	p.Metadata.CPUModel, p.Hostname = "changed", "changed"
	require.Equal(t, "Example CPU", owned.Host.CPUModel)
	require.Equal(t, "native-host", owned.Hostname)
	require.Nil(t, (&Payload{}).CopyCaptureInventory())
}
