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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPublishedDeviceMatchesREDAPLSchema(t *testing.T) {
	data, err := os.ReadFile("testdata/gpu_device_contract.json")
	require.NoError(t, err)
	var contract struct {
		Source           string `json:"source"`
		ResourceType     string `json:"resource_type"`
		HostRelationship struct {
			Source string `json:"source"`
		} `json:"host_relationship"`
		Properties map[string]struct {
			Type     string `json:"type"`
			Required bool   `json:"required"`
		} `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(data, &contract))
	clique := uint32(math.MaxUint32)
	yes, no := true, false
	for _, tc := range []struct {
		name   string
		device Device
	}{
		{name: "required fields", device: Device{UUID: testUUID}},
		{name: "all collected fields", device: Device{
			UUID: testUUID, Vendor: "nvidia", Model: "tesla_t4", Architecture: "turing",
			ComputeCapability: "7.5", CoreCount: 2560, MemoryTotalBytes: 16 << 30,
			MemoryBusWidth: 256, DriverVersion: "580.1", VirtualizationMode: "none",
			PCIBusID: "0000:01:00.0", FabricGUID: "0x0123456789abcdef",
			FabricClusterUUID: "01020304-0506-0708-090a-0b0c0d0e0f10", FabricCliqueID: &clique,
			NVLinkDomain: "01020304-0506-0708-090a-0b0c0d0e0f10:4294967295",
			BoardSerial:  "board", ChassisSerial: "chassis",
			GPUPeers: []GPUPeer{{
				UUID:            "gpu-00000000-0000-0000-0000-000000000002",
				P2PCapabilities: &P2PCapabilities{Read: &yes, Write: &no, Atomics: &yes, NVLink: &yes, PCI: &no},
				DirectNVLinks:   []DirectNVLink{{Port: 0, RemotePCIBusID: "0000:19:00.0", Version: "3.1"}},
			}},
			UnresolvedNVLinks: []UnresolvedNVLink{{Port: math.MaxInt32, RemoteType: "switch", RemotePCIBusID: "0000:80:00.0", Version: "4.0"}},
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
			}
			for name, property := range contract.Properties {
				if property.Required {
					require.Contains(t, fields, name, contract.Source)
				}
			}
			for name, value := range fields {
				property, exists := contract.Properties[name]
				require.True(t, exists, "field %s missing from %s", name, contract.Source)
				switch property.Type {
				case "string":
					var text string
					require.NoError(t, json.Unmarshal(value, &text), name)
					if property.Required {
						require.NotEmpty(t, text, name)
					}
				case "int32", "int64":
					bits := 64
					if property.Type == "int32" {
						bits = 32
					}
					_, err := strconv.ParseInt(string(value), 10, bits)
					require.NoError(t, err, "%s must fit schema type %s", name, property.Type)
				case "array":
					var elements []map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(value, &elements), name)
					requireElementsMatchContract(t, name, elements)
				default:
					t.Fatalf("add contract coverage for emitted %s of type %s", name, property.Type)
				}
			}
			require.JSONEq(t, strconv.Quote(event.Resource.Key), string(fields["gpu_uuid"]))
			require.Equal(t, strings.ToLower(testUUID), event.Resource.Key)
			require.JSONEq(t, `"canonical-host"`, string(fields[strings.TrimPrefix(contract.HostRelationship.Source, ".")]))
		})
	}
}

// arrayElementContract lists the properties of the array items of the gpu_device contract
// (gpu_device.yaml, see the source of gpu_device_contract.json) and which are required. The JSON
// copy of the contract only records that these properties are arrays.
var arrayElementContract = map[string]struct {
	properties map[string]string // name -> type
	required   []string
}{
	"gpu_peers": {
		properties: map[string]string{"gpu_uuid": "string", "p2p_capabilities": "object", "direct_nvlinks": "array"},
		required:   []string{"gpu_uuid"},
	},
	"unresolved_nvlinks": {
		properties: map[string]string{"nvlink_port": "int32", "remote_type": "string", "remote_pci_bus_id": "string", "remote_port": "int32", "version": "string"},
		required:   []string{"nvlink_port"},
	},
}

var nestedElementContract = map[string]struct {
	properties map[string]string
	required   []string
}{
	"gpu_peers.p2p_capabilities": {properties: map[string]string{"read": "boolean", "write": "boolean", "atomics": "boolean", "nvlink": "boolean", "pci": "boolean"}},
	"gpu_peers.direct_nvlinks": {
		properties: map[string]string{"nvlink_port": "int32", "remote_nvlink_port": "int32", "remote_pci_bus_id": "string", "version": "string"},
		required:   []string{"nvlink_port"},
	},
}

func requireElementsMatchContract(t *testing.T, field string, elements []map[string]json.RawMessage) {
	t.Helper()
	contract, ok := arrayElementContract[field]
	require.True(t, ok, "add element contract coverage for emitted array %s", field)
	for _, element := range elements {
		requireObjectMatchesContract(t, field, element, contract.properties, contract.required)
		for name, value := range element {
			if contract.properties[name] == "array" {
				var nested []map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(value, &nested))
				nestedContract := nestedElementContract[field+"."+name]
				for _, item := range nested {
					requireObjectMatchesContract(t, field+"."+name, item, nestedContract.properties, nestedContract.required)
				}
			}
			if contract.properties[name] == "object" {
				var nested map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(value, &nested))
				nestedContract := nestedElementContract[field+"."+name]
				requireObjectMatchesContract(t, field+"."+name, nested, nestedContract.properties, nestedContract.required)
			}
		}
	}
}

func requireObjectMatchesContract(t *testing.T, path string, object map[string]json.RawMessage, properties map[string]string, required []string) {
	t.Helper()
	for _, name := range required {
		require.Contains(t, object, name, "%s: required property", path)
	}
	for name, value := range object {
		kind, ok := properties[name]
		require.True(t, ok, "%s.%s is not in the contract", path, name)
		switch kind {
		case "string":
			var text string
			require.NoError(t, json.Unmarshal(value, &text), "%s.%s", path, name)
		case "boolean":
			var flag bool
			require.NoError(t, json.Unmarshal(value, &flag), "%s.%s", path, name)
		case "int32":
			_, err := strconv.ParseInt(string(value), 10, 32)
			require.NoError(t, err, "%s.%s must fit int32", path, name)
		}
	}
}
