// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	_ "embed"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/assert"
)

//go:embed fixtures/ndm_bytes
var ndmData []byte

func TestNDMAggregator(t *testing.T) {
	t.Run("parseNDMPayload should return empty NDMPayload array on empty data", func(t *testing.T) {
		ndmPayloads, err := ParseNDMPayload(api.Payload{Data: []byte(""), Encoding: encodingEmpty})
		assert.NoError(t, err)
		assert.Empty(t, ndmPayloads)
	})

	t.Run("parseNDMPayload should return empty NDMPayload array on empty json object", func(t *testing.T) {
		ndmPayloads, err := ParseNDMPayload(api.Payload{Data: []byte("{}"), Encoding: encodingJSON})
		assert.NoError(t, err)
		assert.Empty(t, ndmPayloads)
	})

	t.Run("parseNDMPayload should return valid NDMPayload on valid payload", func(t *testing.T) {
		ndmPayloads, err := ParseNDMPayload(api.Payload{Data: ndmData, Encoding: encodingGzip})
		assert.NoError(t, err)
		fmt.Println(len(ndmPayloads))
		assert.Equal(t, len(ndmPayloads), 1)

		ndmPayload := ndmPayloads[0]
		assert.Equal(t, ndmPayload.Namespace, "default")
		assert.Equal(t, string(ndmPayload.Integration), "snmp")
		assert.Equal(t, ndmPayload.Devices[0].ID, "default:127.0.0.1")
		assert.Contains(t, ndmPayload.Devices[0].IDTags, "snmp_device:127.0.0.1")
		assert.Contains(t, ndmPayload.Devices[0].IDTags, "device_namespace:default")
		assert.Contains(t, ndmPayload.Devices[0].Tags, "snmp_profile:cisco-nexus")
		assert.Contains(t, ndmPayload.Devices[0].Tags, "device_vendor:cisco")
		assert.Contains(t, ndmPayload.Devices[0].Tags, "snmp_device:127.0.0.1")
		assert.Contains(t, ndmPayload.Devices[0].Tags, "device_namespace:default")
		assert.Equal(t, ndmPayload.Devices[0].IPAddress, "127.0.0.1")
		assert.Equal(t, int32(ndmPayload.Devices[0].Status), int32(1))
		assert.Equal(t, ndmPayload.Devices[0].Name, "Nexus-eu1.companyname.managed")
		assert.Equal(t, ndmPayload.Devices[0].Description, "oxen acted but acted kept")
		assert.Equal(t, ndmPayload.Devices[0].SysObjectID, "1.3.6.1.4.1.9.12.3.1.3.1.2")
		assert.Equal(t, ndmPayload.Devices[0].Location, "but kept Jaded their but kept quaintly driving their")
		assert.Equal(t, ndmPayload.Devices[0].Profile, "cisco-nexus")
		assert.Equal(t, ndmPayload.Devices[0].Vendor, "cisco")
		assert.Equal(t, ndmPayload.Devices[0].DeviceType, "switch")
		assert.Equal(t, len(ndmPayload.Interfaces), 8)
		assert.Equal(t, ndmPayload.Diagnoses[0].ResourceID, "default:127.0.0.1")
		assert.Equal(t, ndmPayload.Diagnoses[0].ResourceType, "device")
		assert.Equal(t, ndmPayload.CollectTimestamp, int64(1743497402))

		assert.Empty(t, ndmPayload.Subnet)
		assert.Empty(t, ndmPayload.IPAddresses)
		assert.Empty(t, ndmPayload.Links)
		assert.Empty(t, ndmPayload.NetflowExporters)
		assert.Empty(t, ndmPayload.DeviceOIDs)
		assert.Empty(t, ndmPayload.DeviceScanStatus)
	})
}
