// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package payload

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/client"
	"testing"

	"github.com/stretchr/testify/require"

	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

var testInterfaces = []CiscoInterface{
	&VEdgeInterface{
		InterfaceState: client.InterfaceState{
			VmanageSystemIP: "10.0.0.1",
			Ifname:          "test-interface",
			Ifindex:         10,
			SpeedMbps:       "1000",
			IfOperStatus:    "Up",
			IfAdminStatus:   "Down",
			Desc:            "Description",
			Hwaddr:          "00:01:02:03",
			IPAddress:       "10.1.1.5/24",
		},
	},
	&CEdgeInterface{
		CEdgeInterfaceState: client.CEdgeInterfaceState{
			VmanageSystemIP: "10.0.0.2",
			Ifname:          "test-interface",
			Ifindex:         "10",
			SpeedMbps:       "1000",
			IfOperStatus:    "if-oper-state-ready",
			IfAdminStatus:   "if-state-down",
			Description:     "Description",
			Hwaddr:          "00:01:02:03",
			IPAddress:       "10.1.1.5",
			Ipv4SubnetMask:  "255.255.255.0",
			IPV6Address:     "2001:0000:130F:0000:0000:09C0:876A:130B",
		},
	},
}

func TestProcessInterfacesMetadata(t *testing.T) {
	interfaceMetadata, interfaceMap := GetInterfacesMetadata("test-ns", testInterfaces)
	require.Len(t, interfaceMetadata, 2)
	require.Len(t, interfaceMap, 2)
	require.Equal(t, []devicemetadata.InterfaceMetadata{
		{
			DeviceID:    "test-ns:10.0.0.1",
			IDTags:      []string{"interface:test-interface"},
			Index:       10,
			Name:        "test-interface",
			Description: "Description",
			MacAddress:  "00:01:02:03",
			OperStatus:  devicemetadata.OperStatusUp,
			AdminStatus: devicemetadata.AdminStatusDown,
		},
		{
			DeviceID:    "test-ns:10.0.0.2",
			IDTags:      []string{"interface:test-interface"},
			Index:       10,
			Name:        "test-interface",
			Description: "Description",
			MacAddress:  "00:01:02:03",
			OperStatus:  devicemetadata.OperStatusUp,
			AdminStatus: devicemetadata.AdminStatusDown,
		},
	}, interfaceMetadata)
	require.Equal(t, map[string]CiscoInterface{
		"10.0.0.1:test-interface": testInterfaces[0],
		"10.0.0.2:test-interface": testInterfaces[1],
	}, interfaceMap)
}

func TestProcessIPAddressesMetadata(t *testing.T) {
	ipAddressMetadata := GetIPAddressesMetadata("test-ns", testInterfaces)
	require.Len(t, ipAddressMetadata, 3)
	require.Equal(t, []devicemetadata.IPAddressMetadata{
		{
			InterfaceID: "test-ns:10.0.0.1:10",
			IPAddress:   "10.1.1.5",
			Prefixlen:   24,
		},
		{
			InterfaceID: "test-ns:10.0.0.2:10",
			IPAddress:   "10.1.1.5",
			Prefixlen:   24,
		},
		{
			InterfaceID: "test-ns:10.0.0.2:10",
			IPAddress:   "2001:0:130f::9c0:876a:130b",
		},
	}, ipAddressMetadata)
}

func TestConvertOperStatus(t *testing.T) {
	tests := []struct {
		statusMap      map[string]devicemetadata.IfOperStatus
		status         string
		expectedStatus devicemetadata.IfOperStatus
	}{
		{
			status: "up",
			statusMap: map[string]devicemetadata.IfOperStatus{
				"up":   devicemetadata.OperStatusUp,
				"down": devicemetadata.OperStatusDown,
			},
			expectedStatus: devicemetadata.OperStatusUp,
		},
		{
			status: "down",
			statusMap: map[string]devicemetadata.IfOperStatus{
				"up":   devicemetadata.OperStatusUp,
				"down": devicemetadata.OperStatusDown,
			},
			expectedStatus: devicemetadata.OperStatusDown,
		},
		{
			status: "unknown",
			statusMap: map[string]devicemetadata.IfOperStatus{
				"up":   devicemetadata.OperStatusUp,
				"down": devicemetadata.OperStatusDown,
			},
			expectedStatus: devicemetadata.OperStatusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			status := convertOperStatus(tt.statusMap, tt.status)
			require.Equal(t, tt.expectedStatus, status)
		})
	}
}

func TestConvertAdminStatus(t *testing.T) {
	tests := []struct {
		statusMap      map[string]devicemetadata.IfAdminStatus
		status         string
		expectedStatus devicemetadata.IfAdminStatus
	}{
		{
			status: "up",
			statusMap: map[string]devicemetadata.IfAdminStatus{
				"up":   devicemetadata.AdminStatusUp,
				"down": devicemetadata.AdminStatusDown,
			},
			expectedStatus: devicemetadata.AdminStatusUp,
		},
		{
			status: "down",
			statusMap: map[string]devicemetadata.IfAdminStatus{
				"up":   devicemetadata.AdminStatusUp,
				"down": devicemetadata.AdminStatusDown,
			},
			expectedStatus: devicemetadata.AdminStatusDown,
		},
		{
			status: "unknown",
			statusMap: map[string]devicemetadata.IfAdminStatus{
				"up":   devicemetadata.AdminStatusUp,
				"down": devicemetadata.AdminStatusDown,
			},
			expectedStatus: devicemetadata.AdminStatusDown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			status := convertAdminStatus(tt.statusMap, tt.status)
			require.Equal(t, tt.expectedStatus, status)
		})
	}
}
