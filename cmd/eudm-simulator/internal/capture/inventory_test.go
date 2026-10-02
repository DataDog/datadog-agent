// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package capture

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	tc "github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

func TestInventorySanitizesIdentitiesAndPreservesDiscoveryEvidence(t *testing.T) {
	s, err := NewSanitizer()
	if err != nil {
		t.Fatal(err)
	}
	origin := time.Unix(1700000000, 125000000)
	in := &tc.Inventory{Hostname: "PRIVATE-HOST", UUID: "PRIVATE-UUID", Timestamp: origin.Add(time.Second).UnixNano(), Agent: &tc.AgentInventoryMetadata{
		AgentVersion: "7.85.0-localbuild", PackageVersion: "7.85.0-1", Flavor: "agent", InfrastructureMode: "end_user_device",
		AgentStartupTimeMS: origin.Add(-time.Hour).UnixMilli(), FeatureProcessEnabled: true, FeatureNetworksEnabled: true,
	}}
	clean, err := s.Inventory(in, origin)
	if err != nil {
		t.Fatal(err)
	}
	if clean.Hostname != "capture-host" || clean.UUID != s.uuid(in.UUID) || clean.Timestamp != int64(time.Second) || clean.Agent.AgentStartupTimeMS != -time.Hour.Milliseconds() || clean.Agent.InfrastructureMode != "end_user_device" || !clean.Agent.FeatureProcessEnabled || !clean.Agent.FeatureNetworksEnabled || clean.Agent.AgentVersion != in.Agent.AgentVersion {
		t.Fatal("inventory lost its correlated identity, timing, version, or discovery fields")
	}
	clean.Agent.InfrastructureMode = "changed"
	if in.Agent.InfrastructureMode != "end_user_device" || in.Timestamp != origin.Add(time.Second).UnixNano() {
		t.Fatal("sanitizer modified native delivery data")
	}
	in.Agent.InfrastructureMode = "full"
	if _, err := s.Inventory(in, origin); err == nil {
		t.Fatal("non-EUDM inventory was silently converted into EUDM evidence")
	}
}

func TestHostInventoryUsesCrossStreamPseudonymsAndSafeFields(t *testing.T) {
	s, err := NewSanitizer()
	if err != nil {
		t.Fatal(err)
	}
	origin := time.Unix(1700000000, 0)
	in := &tc.Inventory{Hostname: "PRIVATE-SENTINEL", UUID: "PRIVATE-SENTINEL", Timestamp: origin.Add(time.Second).UnixNano(), Host: &tc.HostInventoryMetadata{
		AgentVersion: "7.85.0", OS: "Darwin", OSVersion: "15.6.1", KernelName: "Darwin", KernelRelease: "24.6.0", CPUArchitecture: "arm64",
		CPUCores: 8, CPULogicalProcessors: 8, CPUVendor: "PRIVATE-SENTINEL", CPUModel: "PRIVATE-SENTINEL", CPUModelID: "PRIVATE-SENTINEL", CPUFamily: "PRIVATE-SENTINEL", CPUStepping: "PRIVATE-SENTINEL",
		MemoryTotalKb: 16 << 20, IPAddress: "192.0.2.4", IPv6Address: "2001:db8::4", MacAddress: "00:11:22:33:44:55",
	}}
	clean, err := s.Inventory(in, origin)
	if err != nil {
		t.Fatal(err)
	}
	if clean.Host.IPAddress != s.ip(in.Host.IPAddress) || clean.Host.IPv6Address != s.ip(in.Host.IPv6Address) || clean.Host.MacAddress != s.mac(in.Host.MacAddress) || clean.UUID != s.uuid(in.UUID) {
		t.Fatal("inventory identity pseudonyms diverged from the session sanitizer")
	}
	if clean.Host.MemoryTotalKb != 16<<20 || clean.Host.CPULogicalProcessors != 8 || clean.Host.OS != "Darwin" || clean.Host.OSVersion != "15.6.1" || clean.Host.CPUArchitecture != "arm64" {
		t.Fatal("resource or OS evidence changed")
	}
	body, err := json.Marshal(clean)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"PRIVATE-SENTINEL", "192.0.2.4", "2001:db8::4", "00:11:22:33:44:55"} {
		if strings.Contains(string(body), secret) {
			t.Fatal("native inventory identity persisted")
		}
	}
	clean.Host.OS = "changed"
	if in.Host.OS != "Darwin" {
		t.Fatal("inventory projection aliases production metadata")
	}
}

func TestSystemInfoInventoryPreservesHardwareAndOwnsPseudonyms(t *testing.T) {
	s, err := NewSanitizer()
	if err != nil {
		t.Fatal(err)
	}
	origin := time.Unix(1700000000, 0)
	in := &tc.Inventory{Hostname: "PRIVATE-HOST", UUID: "PRIVATE-UUID", Timestamp: origin.Add(time.Second).UnixNano(), SystemInfo: &tc.HostSystemInfoMetadata{Manufacturer: "Apple Inc.", ModelName: "MacBook Pro", ModelNumber: "PRIVATE-MODEL", SerialNumber: "PRIVATE-SERIAL", ChassisType: "Laptop", Identifier: "Mac16,6"}}
	out, err := s.Inventory(in, origin)
	if err != nil {
		t.Fatal(err)
	}
	if out.SystemInfo.Manufacturer != "Apple Inc." || out.SystemInfo.ModelName != "MacBook Pro" || out.SystemInfo.Identifier != "Mac16,6" || out.SystemInfo.ChassisType != "Laptop" || out.Timestamp != int64(time.Second) {
		t.Fatal("native hardware evidence changed")
	}
	if out.SystemInfo.SerialNumber != s.token("serial_number", in.SystemInfo.SerialNumber) || out.SystemInfo.ModelNumber != s.token("device_model", in.SystemInfo.ModelNumber) {
		t.Fatal("hardware pseudonyms diverged")
	}
	data, _ := json.Marshal(out)
	if strings.Contains(string(data), "PRIVATE-") {
		t.Fatal("native hardware identity escaped")
	}
	out.SystemInfo.Manufacturer = "changed"
	if in.SystemInfo.Manufacturer != "Apple Inc." {
		t.Fatal("projection borrowed native memory")
	}
	in.Host = &tc.HostInventoryMetadata{}
	if _, err := s.Inventory(in, origin); err == nil {
		t.Fatal("mixed inventory kinds accepted")
	}
	in.Host = nil
	in.SystemInfo.ChassisType = "PRIVATE-CHASSIS"
	if _, err := s.Inventory(in, origin); err == nil {
		t.Fatal("unknown chassis accepted")
	}
}
