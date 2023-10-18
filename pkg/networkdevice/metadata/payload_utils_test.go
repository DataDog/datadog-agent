// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package metadata

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_batchPayloads(t *testing.T) {
	collectTime := common.MockTimeNow()
	deviceID := "123"
	devices := []DeviceMetadata{
		{ID: deviceID},
	}

	var interfaces []InterfaceMetadata
	for i := 0; i < 350; i++ {
		interfaces = append(interfaces, InterfaceMetadata{DeviceID: deviceID, Index: int32(i)})
	}
	var ipAddresses []IPAddressMetadata
	for i := 0; i < 100; i++ {
		ipAddresses = append(ipAddresses, IPAddressMetadata{InterfaceID: deviceID + ":1", IPAddress: "1.2.3.4", Prefixlen: 24})
	}
	var topologyLinks []TopologyLinkMetadata
	for i := 0; i < 100; i++ {
		topologyLinks = append(topologyLinks, TopologyLinkMetadata{
			Local:  &TopologyLinkSide{Interface: &TopologyLinkInterface{ID: "a"}},
			Remote: &TopologyLinkSide{Interface: &TopologyLinkInterface{ID: "b"}},
		})
	}
	var netflowExporters []NetflowExporter
	for i := 0; i < 100; i++ {
		netflowExporters = append(netflowExporters, NetflowExporter{
			IPAddress: fmt.Sprintf("1.2.3.%d", i),
			FlowType:  "netflow5",
		})
	}
	var diagnoses []DiagnosisMetadata
	for i := 0; i < 100; i++ {
		diagnoses = append(diagnoses, DiagnosisMetadata{
			ResourceID:   fmt.Sprintf("default:1.2.3.%d", i),
			ResourceType: "device",
			Diagnoses: []Diagnosis{{
				Code:     "TEST_DIAGNOSIS",
				Severity: "warn",
				Message:  "Test diagnosis",
			}},
		})
	}
	payloads := BatchPayloads("my-ns", "127.0.0.0/30", collectTime, 100, devices, interfaces, ipAddresses, topologyLinks, netflowExporters, diagnoses)

	require.Len(t, payloads, 8)

	assert.Equal(t, "my-ns", payloads[0].Namespace)
	assert.Equal(t, "127.0.0.0/30", payloads[0].Subnet)
	assert.Equal(t, int64(946684800), payloads[0].CollectTimestamp)
	assert.Equal(t, devices, payloads[0].Devices)
	assert.Len(t, payloads[0].Interfaces, 99)
	assert.Equal(t, interfaces[0:99], payloads[0].Interfaces)

	assert.Equal(t, "127.0.0.0/30", payloads[1].Subnet)
	assert.Equal(t, int64(946684800), payloads[1].CollectTimestamp)
	assert.Len(t, payloads[1].Devices, 0)
	assert.Len(t, payloads[1].Interfaces, 100)
	assert.Equal(t, interfaces[99:199], payloads[1].Interfaces)

	assert.Len(t, payloads[2].Devices, 0)
	assert.Len(t, payloads[2].Interfaces, 100)
	assert.Equal(t, interfaces[199:299], payloads[2].Interfaces)

	assert.Len(t, payloads[3].Devices, 0)
	assert.Len(t, payloads[3].Interfaces, 51)
	assert.Len(t, payloads[3].IPAddresses, 49)
	assert.Equal(t, interfaces[299:350], payloads[3].Interfaces)
	assert.Equal(t, ipAddresses[:49], payloads[3].IPAddresses)

	assert.Len(t, payloads[4].Devices, 0)
	assert.Len(t, payloads[4].IPAddresses, 51)
	assert.Len(t, payloads[4].Links, 49)
	assert.Equal(t, ipAddresses[49:], payloads[4].IPAddresses)
	assert.Equal(t, topologyLinks[:49], payloads[4].Links)

	assert.Len(t, payloads[5].Devices, 0)
	assert.Len(t, payloads[5].Interfaces, 0)
	assert.Len(t, payloads[5].Links, 51)
	assert.Equal(t, topologyLinks[49:100], payloads[5].Links)
	assert.Equal(t, netflowExporters[:49], payloads[5].NetflowExporters)

	assert.Len(t, payloads[6].Devices, 0)
	assert.Len(t, payloads[6].Interfaces, 0)
	assert.Len(t, payloads[6].Links, 0)
	assert.Len(t, payloads[6].NetflowExporters, 51)
	assert.Equal(t, netflowExporters[49:100], payloads[6].NetflowExporters)
	assert.Len(t, payloads[6].Diagnoses, 49)
	assert.Equal(t, diagnoses[0:49], payloads[6].Diagnoses)

	assert.Len(t, payloads[7].Devices, 0)
	assert.Len(t, payloads[7].Interfaces, 0)
	assert.Len(t, payloads[7].Links, 0)
	assert.Len(t, payloads[7].NetflowExporters, 0)
	assert.Len(t, payloads[7].Diagnoses, 51)
	assert.Equal(t, diagnoses[49:100], payloads[7].Diagnoses)
}
