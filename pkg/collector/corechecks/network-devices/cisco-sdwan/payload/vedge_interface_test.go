// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package payload

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/client"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

func TestVEdgeInterface(t *testing.T) {
	tests := []struct {
		name                string
		namespace           string
		itf                 client.InterfaceState
		expectedID          string
		expectedIndex       int
		expectedSpeed       int
		expectedOperStatus  devicemetadata.IfOperStatus
		expectedAdminStatus devicemetadata.IfAdminStatus
		expectedMetadata    devicemetadata.InterfaceMetadata
		expectedIPV4Address *devicemetadata.IPAddressMetadata
		expectedIPV4Error   string
		expectedIPV6Address *devicemetadata.IPAddressMetadata
		expectedIPV6Error   string
	}{
		{
			name:      "regular interface",
			namespace: "test-ns",
			itf: client.InterfaceState{
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
			expectedID:          "10.0.0.1:test-interface",
			expectedIndex:       10,
			expectedSpeed:       1000,
			expectedOperStatus:  devicemetadata.OperStatusUp,
			expectedAdminStatus: devicemetadata.AdminStatusDown,
			expectedMetadata: devicemetadata.InterfaceMetadata{
				DeviceID:    "test-ns:10.0.0.1",
				IDTags:      []string{"interface:test-interface"},
				Index:       10,
				Name:        "test-interface",
				Description: "Description",
				MacAddress:  "00:01:02:03",
				OperStatus:  devicemetadata.OperStatusUp,
				AdminStatus: devicemetadata.AdminStatusDown,
			},
			expectedIPV4Address: &devicemetadata.IPAddressMetadata{
				InterfaceID: "test-ns:10.0.0.1:10",
				IPAddress:   "10.1.1.5",
				Prefixlen:   24,
			},
		},
		{
			name:      "ipv6 interface",
			namespace: "test-ns",
			itf: client.InterfaceState{
				VmanageSystemIP: "10.0.0.1",
				Ifname:          "test-interface",
				Ifindex:         10,
				SpeedMbps:       "1000",
				IfOperStatus:    "Up",
				IfAdminStatus:   "Down",
				Desc:            "Description",
				Hwaddr:          "00:01:02:03",
				Ipv6Address:     "2001:db8:abcd:0012::0/64",
			},
			expectedID:          "10.0.0.1:test-interface",
			expectedIndex:       10,
			expectedSpeed:       1000,
			expectedOperStatus:  devicemetadata.OperStatusUp,
			expectedAdminStatus: devicemetadata.AdminStatusDown,
			expectedMetadata: devicemetadata.InterfaceMetadata{
				DeviceID:    "test-ns:10.0.0.1",
				IDTags:      []string{"interface:test-interface"},
				Index:       10,
				Name:        "test-interface",
				Description: "Description",
				MacAddress:  "00:01:02:03",
				OperStatus:  devicemetadata.OperStatusUp,
				AdminStatus: devicemetadata.AdminStatusDown,
			},
			expectedIPV6Address: &devicemetadata.IPAddressMetadata{
				InterfaceID: "test-ns:10.0.0.1:10",
				IPAddress:   "2001:db8:abcd:12::",
				Prefixlen:   64,
			},
		},
		{
			name:      "invalid ipv4 address",
			namespace: "test-ns",
			itf: client.InterfaceState{
				VmanageSystemIP: "10.0.0.1",
				Ifname:          "test-interface",
				Ifindex:         10,
				SpeedMbps:       "1000",
				IfOperStatus:    "Up",
				IfAdminStatus:   "Down",
				Desc:            "Description",
				Hwaddr:          "00:01:02:03",
				IPAddress:       "hellohello",
			},
			expectedID:          "10.0.0.1:test-interface",
			expectedIndex:       10,
			expectedSpeed:       1000,
			expectedOperStatus:  devicemetadata.OperStatusUp,
			expectedAdminStatus: devicemetadata.AdminStatusDown,
			expectedMetadata: devicemetadata.InterfaceMetadata{
				DeviceID:    "test-ns:10.0.0.1",
				IDTags:      []string{"interface:test-interface"},
				Index:       10,
				Name:        "test-interface",
				Description: "Description",
				MacAddress:  "00:01:02:03",
				OperStatus:  devicemetadata.OperStatusUp,
				AdminStatus: devicemetadata.AdminStatusDown,
			},
			expectedIPV4Address: nil,
			expectedIPV4Error:   "invalid CIDR address",
		},
		{
			name:      "invalid ipv6 address",
			namespace: "test-ns",
			itf: client.InterfaceState{
				VmanageSystemIP: "10.0.0.1",
				Ifname:          "test-interface",
				Ifindex:         10,
				SpeedMbps:       "1000",
				IfOperStatus:    "Up",
				IfAdminStatus:   "Down",
				Desc:            "Description",
				Hwaddr:          "00:01:02:03",
				Ipv6Address:     "hellohello",
			},
			expectedID:          "10.0.0.1:test-interface",
			expectedIndex:       10,
			expectedSpeed:       1000,
			expectedOperStatus:  devicemetadata.OperStatusUp,
			expectedAdminStatus: devicemetadata.AdminStatusDown,
			expectedMetadata: devicemetadata.InterfaceMetadata{
				DeviceID:    "test-ns:10.0.0.1",
				IDTags:      []string{"interface:test-interface"},
				Index:       10,
				Name:        "test-interface",
				Description: "Description",
				MacAddress:  "00:01:02:03",
				OperStatus:  devicemetadata.OperStatusUp,
				AdminStatus: devicemetadata.AdminStatusDown,
			},
			expectedIPV6Address: nil,
			expectedIPV6Error:   "invalid CIDR address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			itf := VEdgeInterface{tt.itf}

			index, _ := itf.Index()                      // vEdge cannot return errors here
			itfMetadata, _ := itf.Metadata(tt.namespace) // vEdge cannot return errors here

			ipAddress, err := itf.IPV4AddressMetadata(tt.namespace)
			if tt.expectedIPV4Error != "" {
				require.ErrorContains(t, err, tt.expectedIPV4Error)
			} else {
				require.NoError(t, err)
			}

			ipv6Address, err := itf.IPV6AddressMetadata(tt.namespace)
			if tt.expectedIPV6Error != "" {
				require.ErrorContains(t, err, tt.expectedIPV6Error)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tt.expectedID, itf.ID())
			require.Equal(t, tt.expectedIndex, index)
			require.Equal(t, tt.expectedSpeed, itf.GetSpeedMbps())
			require.Equal(t, tt.expectedOperStatus, itf.OperStatus())
			require.Equal(t, tt.expectedAdminStatus, itf.AdminStatus())
			require.Equal(t, tt.expectedMetadata, itfMetadata)
			require.Equal(t, tt.expectedIPV4Address, ipAddress)
			require.Equal(t, tt.expectedIPV6Address, ipv6Address)
		})
	}
}
