// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aggregator

import (
	_ "embed"
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
		assert.Equal(t, 1, len(ndmPayloads))

		ndmPayload := ndmPayloads[0]
		assert.Equal(t, "default", ndmPayload.Namespace)
		assert.Equal(t, "snmp", ndmPayload.Integration)
		assert.Equal(t, "default:127.0.0.1", ndmPayload.Devices[0].ID)
		assert.Contains(t, ndmPayload.Devices[0].IDTags, "snmp_device:127.0.0.1")
		assert.Contains(t, ndmPayload.Devices[0].IDTags, "device_namespace:default")
		assert.Contains(t, ndmPayload.Devices[0].Tags, "snmp_profile:cisco-nexus")
		assert.Contains(t, ndmPayload.Devices[0].Tags, "device_vendor:cisco")
		assert.Contains(t, ndmPayload.Devices[0].Tags, "snmp_device:127.0.0.1")
		assert.Contains(t, ndmPayload.Devices[0].Tags, "device_namespace:default")
		assert.Equal(t, "127.0.0.1", ndmPayload.Devices[0].IPAddress)
		assert.Equal(t, int32(1), ndmPayload.Devices[0].Status)
		assert.Equal(t, "Nexus-eu1.companyname.managed", ndmPayload.Devices[0].Name)
		assert.Equal(t, "oxen acted but acted kept", ndmPayload.Devices[0].Description)
		assert.Equal(t, "1.3.6.1.4.1.9.12.3.1.3.1.2", ndmPayload.Devices[0].SysObjectID)
		assert.Equal(t, "but kept Jaded their but kept quaintly driving their", ndmPayload.Devices[0].Location)
		assert.Equal(t, "cisco-nexus", ndmPayload.Devices[0].Profile)
		assert.Equal(t, "cisco", ndmPayload.Devices[0].Vendor)
		assert.Equal(t, "switch", ndmPayload.Devices[0].DeviceType)
		assert.Equal(t, 8, len(ndmPayload.Interfaces))
		assert.Equal(t, "default:127.0.0.1", ndmPayload.Diagnoses[0].ResourceID)
		assert.Equal(t, "device", ndmPayload.Diagnoses[0].ResourceType)
		assert.Equal(t, int64(1743497402), ndmPayload.CollectTimestamp)
		assert.Empty(t, ndmPayload.Subnet)
	})
}
