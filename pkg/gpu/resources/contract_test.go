// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package resources

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// contractProperty mirrors a gpu_device.yaml property, pinned in testdata.
type contractProperty struct {
	Type       string                      `json:"type"`
	Required   bool                        `json:"required"`
	Enum       []string                    `json:"enum"`
	Items      *contractProperty           `json:"items"`
	Properties map[string]contractProperty `json:"properties"`
}

func TestPublishedDeviceMatchesREDAPLSchema(t *testing.T) {
	data, err := os.ReadFile("testdata/gpu_device_contract.json")
	require.NoError(t, err)
	var contract struct {
		Source           string `json:"source"`
		ResourceType     string `json:"resource_type"`
		HostRelationship struct {
			Source string `json:"source"`
		} `json:"host_relationship"`
		Properties map[string]contractProperty `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(data, &contract))
	clique := uint32(math.MaxUint32)
	numa, slot, tray, module := 0, uint8(0), uint8(255), uint8(7)
	for _, tc := range []struct {
		name   string
		device Device
	}{
		{name: "required fields", device: Device{UUID: testUUID}},
		{name: "all collected fields", device: Device{
			UUID: testUUID, Vendor: "nvidia", Model: "tesla_t4", Architecture: "turing",
			BoardPartNumber: "900-2g183-0000-000", ComputeCapability: "7.5", CoreCount: 2560,
			MemoryTotalBytes: 16 << 30, MemoryBusWidth: 256, BAR1TotalBytes: 256 << 20,
			DriverVersion: "580.1", VirtualizationMode: "none",
			PCIBusID: "0000:01:00.0", PCIDeviceID: "1eb810de", PCISubsystemID: "12a210de",
			PCIePath: &[]string{"0000:00:01.0"}, NUMANode: &numa,
			FabricGUID: "0x0123456789abcdef", FabricClusterUUID: "01020304-0506-0708-090a-0b0c0d0e0f10",
			FabricCliqueID: &clique, NVLinkDomain: "01020304-0506-0708-090a-0b0c0d0e0f10:4294967295",
			NVLinkLinkCount: 18, BoardSerial: "board", ChassisSerial: "chassis",
			SlotNumber: &slot, TrayIndex: &tray, ModuleID: &module, Platform: &Platform{HostID: 1},
			HostLink: &HostLink{
				Protocol: "nvlink_c2c", PCIeMaxGeneration: 6, PCIeGPUMaxGeneration: 6, PCIeMaxWidth: 16,
				PCIeMaxBandwidthBytesPerSecond: 121000000000, C2CLinkCount: 10, C2CMaxBandwidthMbps: 357696,
			},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sender := &recordingSender{}
			require.NoError(t, newTestPublisher(t).Publish(time.Unix(1700000000, 0), "canonical-host", []Device{tc.device}, sender))
			require.Len(t, sender.payloads, 1)
			event, _ := decodeEvent(t, sender.payloads[0])
			require.Equal(t, contract.ResourceType, event.Resource.Type)
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(event.Resource.FieldsJson, &fields))
			if tc.name == "all collected fields" {
				require.Len(t, fields, reflect.TypeOf(Device{}).NumField(), "exercise every published field")
				var hostLink map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(fields["host_link"], &hostLink))
				require.Len(t, hostLink, reflect.TypeOf(HostLink{}).NumField(), "exercise every host_link field")
			}
			requireMatchesContract(t, contract.Source, contract.Properties, fields)
			require.JSONEq(t, strconv.Quote(event.Resource.Key), string(fields["gpu_uuid"]))
			require.Equal(t, strings.ToLower(testUUID), event.Resource.Key)
			require.JSONEq(t, `"canonical-host"`, string(fields[strings.TrimPrefix(contract.HostRelationship.Source, ".")]))
		})
	}
}

func requireMatchesContract(t *testing.T, source string, properties map[string]contractProperty, fields map[string]json.RawMessage) {
	t.Helper()
	for name, property := range properties {
		if property.Required {
			require.Contains(t, fields, name, source)
		}
	}
	for name, value := range fields {
		property, exists := properties[name]
		require.True(t, exists, "field %s missing from %s", name, source)
		requireValueMatches(t, source, name, property, value)
	}
}

func requireValueMatches(t *testing.T, source, name string, property contractProperty, value json.RawMessage) {
	t.Helper()
	switch property.Type {
	case "string":
		var text string
		require.NoError(t, json.Unmarshal(value, &text), name)
		if property.Required {
			require.NotEmpty(t, text, name)
		}
		if len(property.Enum) > 0 {
			require.True(t, slices.Contains(property.Enum, text), "%s=%q is not in %v", name, text, property.Enum)
		}
	case "int32", "int64":
		bits := 64
		if property.Type == "int32" {
			bits = 32
		}
		_, err := strconv.ParseInt(string(value), 10, bits)
		require.NoError(t, err, "%s must fit schema type %s", name, property.Type)
	case "array":
		require.NotNil(t, property.Items, "add item coverage for %s", name)
		var items []json.RawMessage
		require.NoError(t, json.Unmarshal(value, &items), name)
		for _, item := range items {
			requireValueMatches(t, source, name+"[]", *property.Items, item)
		}
	case "object":
		require.NotEmpty(t, property.Properties, "add property coverage for %s", name)
		var nested map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(value, &nested), name)
		requireMatchesContract(t, source, property.Properties, nested)
	default:
		t.Fatalf("add contract coverage for emitted %s of type %s", name, property.Type)
	}
}
