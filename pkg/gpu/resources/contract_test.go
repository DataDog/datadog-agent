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
