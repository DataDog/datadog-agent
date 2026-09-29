// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package metadata

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/integrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchPayloadsWithWirelessInterfaces(t *testing.T) {
	at := mockTimeNow()
	devices := []DeviceMetadata{{ID: DeviceID("run", "192.0.2.1")}}
	interfaces := []InterfaceMetadata{{DeviceID: devices[0].ID, Index: 1}}
	wireless := make([]WirelessInterfaceMetadata, 205)
	for i := range wireless {
		wireless[i] = WirelessInterfaceMetadata{Namespace: "run", DeviceByIntegrationID: devices[0].ID, InterfaceByIntegrationID: InterfaceID(devices[0].ID, int32(i+1)), BSSID: fmt.Sprintf("02:00:00:00:00:%02x", i), SSID: "ssid", Band: "5GHz", AdminStatus: AdminStatusUp, OperStatus: OperStatusUp}
	}
	addresses := []IPAddressMetadata{{InterfaceID: InterfaceID(devices[0].ID, 1), IPAddress: "192.0.2.1"}}
	for _, size := range []int{1, 100} {
		payloads := BatchPayloadsWithWirelessInterfaces(integrations.SNMP, "run", "", at, size, devices, interfaces, wireless, addresses, nil, nil, nil, nil)
		var actual []WirelessInterfaceMetadata
		total := 0
		for _, payload := range payloads {
			count := len(payload.Devices) + len(payload.Interfaces) + len(payload.WirelessInterfaces) + len(payload.IPAddresses)
			require.Positive(t, count)
			require.LessOrEqual(t, count, size)
			assert.Equal(t, "run", payload.Namespace)
			assert.Equal(t, integrations.SNMP, payload.Integration)
			assert.Equal(t, at.Unix(), payload.CollectTimestamp)
			total += count
			actual = append(actual, payload.WirelessInterfaces...)
		}
		assert.Equal(t, len(devices)+len(interfaces)+len(wireless)+len(addresses), total)
		assert.Equal(t, wireless, actual)
	}

	legacy := BatchPayloads(integrations.SNMP, "run", "", at, 100, devices, interfaces, addresses, nil, nil, nil, nil)
	withNoWireless := BatchPayloadsWithWirelessInterfaces(integrations.SNMP, "run", "", at, 100, devices, interfaces, nil, addresses, nil, nil, nil, nil)
	assert.Equal(t, legacy, withNoWireless)
	encoded, err := json.Marshal(legacy)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "wireless_interfaces")
	encoded, err = json.Marshal(BatchPayloadsWithWirelessInterfaces(integrations.SNMP, "run", "", at, 100, nil, nil, wireless[:1], nil, nil, nil, nil, nil))
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"wireless_interfaces"`)
	assert.Contains(t, string(encoded), `"ndm_device_by_integration_id":"run:192.0.2.1"`)
	assert.Contains(t, string(encoded), `"ndm_interface_by_integration_id":"run:192.0.2.1:1"`)
}

func TestNDMIdentityHelpers(t *testing.T) {
	deviceID := DeviceID("run", "192.0.2.1")
	assert.Equal(t, "run:192.0.2.1", deviceID)
	assert.Equal(t, []string{"device_namespace:run", "snmp_device:192.0.2.1"}, DeviceIDTags("run", "192.0.2.1"))
	assert.Equal(t, "run:192.0.2.1:2", InterfaceID(deviceID, 2))
	assert.Equal(t, []string{"interface:radio", "interface_index:2"}, InterfaceIDTags("radio", 2))
	assert.Equal(t, "dd.internal.resource:ndm_device:run:192.0.2.1", DeviceResourceTag(deviceID))
	assert.Equal(t, "dd.internal.resource:ndm_interface:run:192.0.2.1:2", InterfaceResourceTag(deviceID, 2))
}

// mockTimeNow mocks time.Now
var mockTimeNow = func() time.Time {
	layout := "2006-01-02 15:04:05"
	str := "2000-01-01 00:00:00"
	t, _ := time.Parse(layout, str)
	return t
}

func Test_batchPayloads(t *testing.T) {
	collectTime := mockTimeNow()
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
	var vpnTunnels []VPNTunnelMetadata
	for i := 0; i < 100; i++ {
		vpnTunnels = append(vpnTunnels, VPNTunnelMetadata{
			DeviceID:        deviceID,
			InterfaceID:     deviceID + ":1",
			LocalOutsideIP:  "1.2.3.4",
			RemoteOutsideIP: "4.3.2.1",
			Protocol:        "ipsec",
			RouteAddresses:  []string{"192.168.0.0/24", "10.0.0.0/16"},
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
	payloads := BatchPayloads("test-integration", "my-ns", "127.0.0.0/30", collectTime, 100, devices, interfaces, ipAddresses, topologyLinks, vpnTunnels, netflowExporters, diagnoses)

	require.Len(t, payloads, 9)

	assert.Equal(t, integrations.Integration("test-integration"), payloads[0].Integration)
	assert.Equal(t, "my-ns", payloads[0].Namespace)
	assert.Equal(t, "127.0.0.0/30", payloads[0].Subnet)
	assert.Equal(t, int64(946684800), payloads[0].CollectTimestamp)
	assert.Equal(t, devices, payloads[0].Devices)
	assert.Len(t, payloads[0].Interfaces, 99)
	assert.Equal(t, interfaces[0:99], payloads[0].Interfaces)

	assert.Equal(t, integrations.Integration("test-integration"), payloads[1].Integration)
	assert.Equal(t, "127.0.0.0/30", payloads[1].Subnet)
	assert.Equal(t, int64(946684800), payloads[1].CollectTimestamp)
	assert.Len(t, payloads[1].Devices, 0)
	assert.Len(t, payloads[1].Interfaces, 100)
	assert.Equal(t, interfaces[99:199], payloads[1].Interfaces)

	assert.Equal(t, integrations.Integration("test-integration"), payloads[2].Integration)
	assert.Len(t, payloads[2].Devices, 0)
	assert.Len(t, payloads[2].Interfaces, 100)
	assert.Equal(t, interfaces[199:299], payloads[2].Interfaces)

	assert.Equal(t, integrations.Integration("test-integration"), payloads[3].Integration)
	assert.Len(t, payloads[3].Devices, 0)
	assert.Len(t, payloads[3].Interfaces, 51)
	assert.Len(t, payloads[3].IPAddresses, 49)
	assert.Equal(t, interfaces[299:350], payloads[3].Interfaces)
	assert.Equal(t, ipAddresses[:49], payloads[3].IPAddresses)

	assert.Equal(t, integrations.Integration("test-integration"), payloads[4].Integration)
	assert.Len(t, payloads[4].Devices, 0)
	assert.Len(t, payloads[4].IPAddresses, 51)
	assert.Len(t, payloads[4].Links, 49)
	assert.Equal(t, ipAddresses[49:], payloads[4].IPAddresses)
	assert.Equal(t, topologyLinks[:49], payloads[4].Links)

	assert.Equal(t, integrations.Integration("test-integration"), payloads[5].Integration)
	assert.Len(t, payloads[5].Devices, 0)
	assert.Len(t, payloads[5].Interfaces, 0)
	assert.Len(t, payloads[5].IPAddresses, 0)
	assert.Len(t, payloads[5].Links, 51)
	assert.Len(t, payloads[5].VPNTunnels, 49)
	assert.Equal(t, topologyLinks[49:100], payloads[5].Links)
	assert.Equal(t, vpnTunnels[:49], payloads[5].VPNTunnels)

	assert.Equal(t, integrations.Integration("test-integration"), payloads[6].Integration)
	assert.Len(t, payloads[6].Devices, 0)
	assert.Len(t, payloads[6].Interfaces, 0)
	assert.Len(t, payloads[6].Links, 0)
	assert.Len(t, payloads[6].VPNTunnels, 51)
	assert.Len(t, payloads[6].NetflowExporters, 49)
	assert.Equal(t, vpnTunnels[49:100], payloads[6].VPNTunnels)
	assert.Equal(t, netflowExporters[:49], payloads[6].NetflowExporters)

	assert.Equal(t, integrations.Integration("test-integration"), payloads[7].Integration)
	assert.Len(t, payloads[7].Devices, 0)
	assert.Len(t, payloads[7].Interfaces, 0)
	assert.Len(t, payloads[7].Links, 0)
	assert.Len(t, payloads[7].VPNTunnels, 0)
	assert.Len(t, payloads[7].NetflowExporters, 51)
	assert.Equal(t, netflowExporters[49:100], payloads[7].NetflowExporters)
	assert.Len(t, payloads[7].Diagnoses, 49)
	assert.Equal(t, diagnoses[0:49], payloads[7].Diagnoses)

	assert.Equal(t, integrations.Integration("test-integration"), payloads[8].Integration)
	assert.Len(t, payloads[8].Devices, 0)
	assert.Len(t, payloads[8].Interfaces, 0)
	assert.Len(t, payloads[8].Links, 0)
	assert.Len(t, payloads[8].VPNTunnels, 0)
	assert.Len(t, payloads[8].NetflowExporters, 0)
	assert.Len(t, payloads[8].Diagnoses, 51)
	assert.Equal(t, diagnoses[49:100], payloads[8].Diagnoses)
}
