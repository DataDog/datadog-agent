// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetrycapture

import (
	"strings"
	"testing"
	"time"
	"unsafe"
)

func TestPrepareSupportsAllDeclaredStreams(t *testing.T) {
	streams := []Stream{Metrics, Metadata, Processes, Connections, Software, AgentInventory, HostInventory, HostSystemInfo}
	m := testManager(t, streams...)
	arm(t, m, streams...)
	if m.Status().State != Active {
		t.Fatal("declared streams could not be activated")
	}
}

func TestInventoryQueueRequiresMatchingSingleProjection(t *testing.T) {
	for _, stream := range []Stream{AgentInventory, HostInventory, HostSystemInfo} {
		t.Run(string(stream), func(t *testing.T) {
			m := testManager(t, stream)
			arm(t, m, stream)
			input := &Inventory{Hostname: "native-host", UUID: "native-uuid", Timestamp: time.Now().UnixNano()}
			switch stream {
			case AgentInventory:
				input.Agent = &AgentInventoryMetadata{AgentVersion: "7.85.0", FeatureProcessEnabled: true}
			case HostInventory:
				input.Host = &HostInventoryMetadata{OS: "Darwin", MemoryTotalKb: 123456}
			case HostSystemInfo:
				input.SystemInfo = &HostSystemInfoMetadata{Manufacturer: "Example", SerialNumber: "private-device"}
			}
			p := Payload{Inventory: input}
			if !m.Observe(stream, time.Now(), time.Minute, PayloadSize(p), func() Payload { return Payload{Inventory: CloneInventory(input)} }) {
				t.Fatal("valid inventory rejected")
			}
			input.Hostname = "changed"
			batch, err := m.Read(ReadRequest{Control: testControl})
			if err != nil {
				t.Fatal(err)
			}
			if len(batch.Records) != 1 || batch.Records[0].Payload.Inventory.Hostname != "native-host" {
				t.Fatal("inventory record lost ownership")
			}
			batch.Release()
			input.Agent, input.Host = &AgentInventoryMetadata{}, &HostInventoryMetadata{}
			if m.Observe(stream, time.Now(), time.Minute, PayloadSize(p), func() Payload { return p }) || m.Status().State != Failed {
				t.Fatal("ambiguous inventory accepted")
			}
			assertEmpty(t, m)
		})
	}
}

func TestInventoryRejectsEveryWrongOrAmbiguousProjection(t *testing.T) {
	for _, stream := range []Stream{AgentInventory, HostInventory, HostSystemInfo} {
		for mask := 0; mask < 8; mask++ {
			i := &Inventory{}
			if mask&1 != 0 {
				i.Agent = &AgentInventoryMetadata{}
			}
			if mask&2 != 0 {
				i.Host = &HostInventoryMetadata{}
			}
			if mask&4 != 0 {
				i.SystemInfo = &HostSystemInfoMetadata{}
			}
			want := (stream == AgentInventory && mask == 1) || (stream == HostInventory && mask == 2) || (stream == HostSystemInfo && mask == 4)
			if validPayload(stream, Payload{Inventory: i}) != want {
				t.Fatalf("projection mask %d incorrectly validated for %s", mask, stream)
			}
		}
	}
}

func TestSystemInfoInventoryOwnsAndChargesAllStrings(t *testing.T) {
	i := &Inventory{Hostname: "native-host", UUID: "native-uuid", SystemInfo: &HostSystemInfoMetadata{
		Manufacturer: "manufacturer", ModelNumber: "model-number", SerialNumber: "serial-number",
		ModelName: "model-name", ChassisType: "chassis-type", Identifier: "identifier",
	}}
	base := InventorySize(&Inventory{SystemInfo: &HostSystemInfoMetadata{}})
	want := base + int64(len(i.Hostname)+len(i.UUID))
	for _, field := range hostSystemInfoStrings(i.SystemInfo) {
		want += int64(len(*field))
	}
	if InventorySize(i) != want {
		t.Fatal("hardware inventory does not charge every retained string")
	}
	owned := CloneInventory(i)
	if owned.SystemInfo == i.SystemInfo || owned.Agent != nil || owned.Host != nil {
		t.Fatal("hardware projection ownership or union changed")
	}
	for n, field := range hostSystemInfoStrings(i.SystemInfo) {
		copy := hostSystemInfoStrings(owned.SystemInfo)[n]
		if *field != *copy || unsafe.StringData(*field) == unsafe.StringData(*copy) {
			t.Fatal("hardware field does not own its string storage")
		}
	}
	m := testManager(t, HostSystemInfo)
	arm(t, m, HostSystemInfo)
	i.SystemInfo.SerialNumber = strings.Repeat("x", int(MaxItemBytes))
	p := Payload{Inventory: i}
	copied := false
	if m.Observe(HostSystemInfo, time.Now(), time.Hour, PayloadSize(p), func() Payload { copied = true; return p }) || copied {
		t.Fatal("oversize hardware projection copied before reservation")
	}
	assertEmpty(t, m)
}

func TestInventoryOversizeReservationDoesNotCopy(t *testing.T) {
	m := testManager(t, AgentInventory)
	arm(t, m, AgentInventory)
	p := Payload{Inventory: &Inventory{Agent: &AgentInventoryMetadata{AgentVersion: strings.Repeat("x", int(MaxItemBytes))}}}
	called := false
	if m.Observe(AgentInventory, time.Now(), time.Minute, PayloadSize(p), func() Payload { called = true; return p }) || called {
		t.Fatal("oversize inventory was copied")
	}
	if m.Status().State != Failed {
		t.Fatal("overflow did not disarm capture")
	}
	assertEmpty(t, m)
}
