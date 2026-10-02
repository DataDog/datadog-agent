// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package capture

import (
	"reflect"
	"testing"
	"time"

	tc "github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

func TestAgentInventoryPreservesIdentityAndRelativeUptime(t *testing.T) {
	origin := time.Unix(1700000000, 125000000)
	in := &tc.Inventory{Hostname: "engineer-laptop", UUID: "device-uuid", Timestamp: origin.Add(time.Second).UnixNano(), Agent: &tc.AgentInventoryMetadata{
		AgentVersion: "7.85.0-localbuild", PackageVersion: "7.85.0~rc1+local", Flavor: "agent", InfrastructureMode: "end_user_device",
		AgentStartupTimeMS: origin.Add(-time.Hour).UnixMilli(), FeatureProcessEnabled: true, FeatureNetworksEnabled: true,
	}}
	out, err := NewNormalizer().Inventory(in, origin)
	if err != nil {
		t.Fatal(err)
	}
	want := tc.CloneInventory(in)
	want.Timestamp, want.Agent.AgentStartupTimeMS = int64(time.Second), -time.Hour.Milliseconds()
	if !reflect.DeepEqual(out, want) {
		t.Fatal("inventory changed identity, feature values, or timing")
	}
	out.Agent.InfrastructureMode = "changed"
	if in.Agent.InfrastructureMode != "end_user_device" || in.Timestamp != origin.Add(time.Second).UnixNano() {
		t.Fatal("inventory copy aliases producer memory")
	}
	in.Agent.InfrastructureMode = "full"
	if _, err := NewNormalizer().Inventory(in, origin); err == nil {
		t.Fatal("non-EUDM inventory silently converted to EUDM")
	}
}

func TestHostAndSystemInventoryPreserveNativeHardware(t *testing.T) {
	origin := time.Unix(1700000000, 0)
	for _, in := range []*tc.Inventory{
		{Hostname: "engineer-laptop", UUID: "device-uuid", Timestamp: origin.Add(time.Second).UnixNano(), Host: &tc.HostInventoryMetadata{
			AgentVersion: "7.85.0", OS: "Windows", OSVersion: "Windows 11 Pro 24H2", KernelName: "Windows", KernelRelease: "10.0.26100", CPUArchitecture: "amd64",
			CPUCores: 8, CPULogicalProcessors: 16, CPUVendor: "GenuineIntel", CPUModel: "Intel(R) Core(TM) Ultra 7 155H", CPUModelID: "0xAB", CPUFamily: "Intel64 Family 6", CPUStepping: "B0",
			MemoryTotalKb: 16 << 20, IPAddress: "192.0.2.4", IPv6Address: "2001:db8::4", MacAddress: "00:11:22:33:44:55",
		}},
		{Hostname: "engineer-laptop", UUID: "device-uuid", Timestamp: origin.Add(time.Second).UnixNano(), SystemInfo: &tc.HostSystemInfoMetadata{Manufacturer: "Framework", ModelName: "Laptop 13", ModelNumber: "FRANBMCP07", SerialNumber: "FRANBMCP071234", ChassisType: "Notebook", Identifier: "native-device-id"}},
	} {
		out, err := NewNormalizer().Inventory(in, origin)
		if err != nil {
			t.Fatal(err)
		}
		want := tc.CloneInventory(in)
		want.Timestamp = int64(time.Second)
		if !reflect.DeepEqual(out, want) {
			t.Fatal("inventory discarded native hardware fields")
		}
		if out.Host != nil {
			out.Host.CPUModel = "changed"
			if in.Host.CPUModel != "Intel(R) Core(TM) Ultra 7 155H" {
				t.Fatal("host inventory copy aliases producer memory")
			}
		} else {
			out.SystemInfo.SerialNumber = "changed"
			if in.SystemInfo.SerialNumber != "FRANBMCP071234" {
				t.Fatal("system inventory copy aliases producer memory")
			}
		}
	}
	invalid := &tc.Inventory{Hostname: "host", UUID: "id", Timestamp: origin.UnixNano(), Agent: &tc.AgentInventoryMetadata{}, Host: &tc.HostInventoryMetadata{}}
	if _, err := NewNormalizer().Inventory(invalid, origin); err == nil {
		t.Fatal("mixed inventory kinds accepted")
	}
}
