// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetry

import (
	"encoding/json"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

func TestInventoryDecodeRequiresObservedRegistrationAndHardware(t *testing.T) {
	agent := &telemetrycapture.Inventory{Hostname: "capture-host", UUID: "capture-uuid", Timestamp: 1_000_000_123,
		Agent: &telemetrycapture.AgentInventoryMetadata{AgentVersion: "7.85.0", Flavor: "agent", InfrastructureMode: "end_user_device", AgentStartupTimeMS: -3000}}
	host := &telemetrycapture.Inventory{Hostname: "capture-host", UUID: "capture-uuid", Timestamp: 2_000_000_456,
		Host: &telemetrycapture.HostInventoryMetadata{AgentVersion: "7.85.0", OS: "Darwin", CPUCores: 4, CPULogicalProcessors: 8, MemoryTotalKb: 8 << 20,
			IPAddress: "192.0.2.1", IPv6Address: "2001:db8::1", MacAddress: "02:00:00:00:00:01"}}
	for _, test := range []struct {
		name   string
		stream schema.Stream
		value  *telemetrycapture.Inventory
		mutate func(*telemetrycapture.Inventory)
	}{
		{"Agent", schema.AgentInventory, agent, nil},
		{"host with optional architecture absent", schema.HostInventory, host, nil},
		{"wrong stream", schema.HostInventory, agent, func(*telemetrycapture.Inventory) {}},
		{"both metadata types", schema.AgentInventory, agent, func(v *telemetrycapture.Inventory) { v.Host = host.Host }},
		{"missing hostname", schema.HostInventory, host, func(v *telemetrycapture.Inventory) { v.Hostname = "" }},
		{"missing uuid", schema.AgentInventory, agent, func(v *telemetrycapture.Inventory) { v.UUID = "" }},
		{"negative timestamp", schema.AgentInventory, agent, func(v *telemetrycapture.Inventory) { v.Timestamp = -1 }},
		{"wrong mode", schema.AgentInventory, agent, func(v *telemetrycapture.Inventory) { v.Agent.InfrastructureMode = "infrastructure" }},
		{"missing version", schema.AgentInventory, agent, func(v *telemetrycapture.Inventory) { v.Agent.AgentVersion = "" }},
		{"future startup", schema.AgentInventory, agent, func(v *telemetrycapture.Inventory) { v.Agent.AgentStartupTimeMS = 1001 }},
		{"kernel OS mismatch", schema.HostInventory, host, func(v *telemetrycapture.Inventory) { v.Host.KernelName = "Windows" }},
		{"wrong OS", schema.HostInventory, host, func(v *telemetrycapture.Inventory) { v.Host.OS = "linux" }},
		{"impossible cores", schema.HostInventory, host, func(v *telemetrycapture.Inventory) { v.Host.CPULogicalProcessors = 2 }},
		{"missing memory", schema.HostInventory, host, func(v *telemetrycapture.Inventory) { v.Host.MemoryTotalKb = 0 }},
		{"wrong IPv4 family", schema.HostInventory, host, func(v *telemetrycapture.Inventory) { v.Host.IPAddress = "2001:db8::1" }},
		{"wrong IPv6 family", schema.HostInventory, host, func(v *telemetrycapture.Inventory) { v.Host.IPv6Address = "192.0.2.1" }},
		{"invalid MAC", schema.HostInventory, host, func(v *telemetrycapture.Inventory) { v.Host.MacAddress = "invalid" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := telemetrycapture.CloneInventory(test.value)
			if test.mutate != nil {
				test.mutate(value)
			}
			data, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := Decode(test.stream, data)
			if test.mutate != nil {
				if err == nil {
					t.Fatal("accepted invalid inventory")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Inventory.Timestamp != value.Timestamp || decoded.Inventory.UUID != value.UUID {
				t.Fatal("inventory identity or fractional collection timestamp changed")
			}
		})
	}
}

func TestInventoryDecodeRejectsConfigurationAndRemovedIdentifiers(t *testing.T) {
	for _, field := range []string{"agent_config", "api_key", "dmi_product_uuid", "hypervisor_guest_uuid", "kernel_version"} {
		data := []byte(`{"hostname":"capture-host","uuid":"capture-uuid","timestamp":0,"agent_metadata":{"agent_version":"7.85.0","flavor":"agent","infrastructure_mode":"end_user_device","` + field + `":"sensitive"}}`)
		if _, err := Decode(schema.AgentInventory, data); err == nil {
			t.Fatalf("accepted %s", field)
		}
	}
}

func TestInventoryPlatformAcceptsNativeWindowsDescriptions(t *testing.T) {
	for _, value := range []string{"Microsoft Windows 11 Pro", "Microsoft Windows 10 Enterprise", "Windows Server 2025 Datacenter", "Darwin", "macOS"} {
		if InventoryPlatform(value) == "" {
			t.Fatalf("rejected native operating system %q", value)
		}
	}
	for _, value := range []string{"Windows customer-owned-device", "Microsoft Windows 11 Pro private-host", "linux"} {
		if InventoryPlatform(value) != "" {
			t.Fatalf("accepted unsupported or unbounded description %q", value)
		}
	}
}

func TestSystemInfoInventoryDecode(t *testing.T) {
	value := &telemetrycapture.Inventory{Hostname: "capture-host", UUID: "capture-uuid", Timestamp: 1, SystemInfo: &telemetrycapture.HostSystemInfoMetadata{Manufacturer: "Apple Inc.", ChassisType: "Laptop"}}
	for _, stream := range []schema.Stream{schema.HostSystemInfo, schema.HostInventory, schema.AgentInventory} {
		data, _ := json.Marshal(value)
		_, err := Decode(stream, data)
		if (err == nil) != (stream == schema.HostSystemInfo) {
			t.Fatalf("wrong inventory variant accepted: %s", stream)
		}
	}
	value.SystemInfo.Manufacturer = ""
	data, _ := json.Marshal(value)
	if _, err := Decode(schema.HostSystemInfo, data); err == nil {
		t.Fatal("empty hardware evidence accepted")
	}
}
