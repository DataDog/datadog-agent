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

func boolPtr(b bool) *bool {
	return &b
}

func TestCEdgeInterface(t *testing.T) {
	tests := []struct {
		name                     string
		namespace                string
		itf                      client.CEdgeInterfaceState
		expectedID               string
		expectedIndex            int32
		expectedIndexError       string
		expectedSpeed            float64
		expectedOperStatus       devicemetadata.IfOperStatus
		expectedAdminStatus      devicemetadata.IfAdminStatus
		expectedMetadata         devicemetadata.InterfaceMetadata
		expectedInterfaceError   string
		expectedIPV4Address      *devicemetadata.IPAddressMetadata
		expectedIPV4AddressError string
		expectedIPV6Address      *devicemetadata.IPAddressMetadata
		expectedIPV6AddressError string
	}{
		{
			name:      "regular interface",
			namespace: "test-ns",
			itf: client.CEdgeInterfaceState{
				VmanageSystemIP: "10.0.0.1",
				Ifname:          "test-interface",
				Ifindex:         "10",
				SpeedMbps:       "1000",
				IfOperStatus:    "if-oper-state-ready",
				IfAdminStatus:   "if-state-down",
				Description:     "Description",
				Hwaddr:          "00:01:02:03",
				IPAddress:       "10.1.1.5",
				Ipv4SubnetMask:  "255.255.255.0",
				IPV6Address:     "2001:db8:abcd:0012::0",
				InterfaceType:   "iana-iftype-ethernet-csmacd",
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
				Type:        6,
				IsPhysical:  boolPtr(true),
			},
			expectedIPV4Address: &devicemetadata.IPAddressMetadata{
				InterfaceID: "test-ns:10.0.0.1:10",
				IPAddress:   "10.1.1.5",
				Prefixlen:   24,
			},
			expectedIPV6Address: &devicemetadata.IPAddressMetadata{
				InterfaceID: "test-ns:10.0.0.1:10",
				IPAddress:   "2001:db8:abcd:12::",
			},
		},
		{
			name:      "loopback interface",
			namespace: "test-ns",
			itf: client.CEdgeInterfaceState{
				VmanageSystemIP: "10.0.0.1",
				Ifname:          "lo0",
				Ifindex:         "20",
				SpeedMbps:       "0",
				IfOperStatus:    "if-oper-state-ready",
				IfAdminStatus:   "if-state-up",
				Description:     "Loopback",
				InterfaceType:   "iana-iftype-sw-loopback",
			},
			expectedID:          "10.0.0.1:lo0",
			expectedIndex:       20,
			expectedSpeed:       0,
			expectedOperStatus:  devicemetadata.OperStatusUp,
			expectedAdminStatus: devicemetadata.AdminStatusUp,
			expectedMetadata: devicemetadata.InterfaceMetadata{
				DeviceID:    "test-ns:10.0.0.1",
				IDTags:      []string{"interface:lo0"},
				Index:       20,
				Name:        "lo0",
				Description: "Loopback",
				OperStatus:  devicemetadata.OperStatusUp,
				AdminStatus: devicemetadata.AdminStatusUp,
				Type:        24,
				IsPhysical:  boolPtr(false),
			},
		},
		{
			name:      "tunnel interface",
			namespace: "test-ns",
			itf: client.CEdgeInterfaceState{
				VmanageSystemIP: "10.0.0.1",
				Ifname:          "tun0",
				Ifindex:         "30",
				SpeedMbps:       "100",
				IfOperStatus:    "if-oper-state-ready",
				IfAdminStatus:   "if-state-up",
				Description:     "Tunnel",
				InterfaceType:   "iana-iftype-tunnel",
			},
			expectedID:          "10.0.0.1:tun0",
			expectedIndex:       30,
			expectedSpeed:       100,
			expectedOperStatus:  devicemetadata.OperStatusUp,
			expectedAdminStatus: devicemetadata.AdminStatusUp,
			expectedMetadata: devicemetadata.InterfaceMetadata{
				DeviceID:    "test-ns:10.0.0.1",
				IDTags:      []string{"interface:tun0"},
				Index:       30,
				Name:        "tun0",
				Description: "Tunnel",
				OperStatus:  devicemetadata.OperStatusUp,
				AdminStatus: devicemetadata.AdminStatusUp,
				Type:        131,
				IsPhysical:  boolPtr(false),
			},
		},
		{
			name:      "unknown interface type",
			namespace: "test-ns",
			itf: client.CEdgeInterfaceState{
				VmanageSystemIP: "10.0.0.1",
				Ifname:          "unknown0",
				Ifindex:         "40",
				SpeedMbps:       "100",
				IfOperStatus:    "if-oper-state-ready",
				IfAdminStatus:   "if-state-up",
				Description:     "Unknown",
				InterfaceType:   "unknown-type",
			},
			expectedID:          "10.0.0.1:unknown0",
			expectedIndex:       40,
			expectedSpeed:       100,
			expectedOperStatus:  devicemetadata.OperStatusUp,
			expectedAdminStatus: devicemetadata.AdminStatusUp,
			expectedMetadata: devicemetadata.InterfaceMetadata{
				DeviceID:    "test-ns:10.0.0.1",
				IDTags:      []string{"interface:unknown0"},
				Index:       40,
				Name:        "unknown0",
				Description: "Unknown",
				OperStatus:  devicemetadata.OperStatusUp,
				AdminStatus: devicemetadata.AdminStatusUp,
				Type:        0,
				IsPhysical:  boolPtr(false),
			},
		},
		{
			name:      "other interface type",
			namespace: "test-ns",
			itf: client.CEdgeInterfaceState{
				VmanageSystemIP: "10.0.0.1",
				Ifname:          "other0",
				Ifindex:         "50",
				SpeedMbps:       "100",
				IfOperStatus:    "if-oper-state-ready",
				IfAdminStatus:   "if-state-up",
				Description:     "Other",
				InterfaceType:   "iana-iftype-other",
			},
			expectedID:          "10.0.0.1:other0",
			expectedIndex:       50,
			expectedSpeed:       100,
			expectedOperStatus:  devicemetadata.OperStatusUp,
			expectedAdminStatus: devicemetadata.AdminStatusUp,
			expectedMetadata: devicemetadata.InterfaceMetadata{
				DeviceID:    "test-ns:10.0.0.1",
				IDTags:      []string{"interface:other0"},
				Index:       50,
				Name:        "other0",
				Description: "Other",
				OperStatus:  devicemetadata.OperStatusUp,
				AdminStatus: devicemetadata.AdminStatusUp,
				Type:        1,
				IsPhysical:  boolPtr(false),
			},
		},
		{
			name:      "invalid index",
			namespace: "test-ns",
			itf: client.CEdgeInterfaceState{
				VmanageSystemIP: "10.0.0.1",
				Ifname:          "test-interface",
				Ifindex:         "iamnotanindex",
				SpeedMbps:       "1000",
				IfOperStatus:    "if-oper-state-ready",
				IfAdminStatus:   "if-state-down",
				Description:     "Description",
				Hwaddr:          "00:01:02:03",
				IPAddress:       "10.1.1.5",
				Ipv4SubnetMask:  "255.255.255.0",
				IPV6Address:     "2001:db8:abcd:0012::0",
				InterfaceType:   "iana-iftype-ethernet-csmacd",
			},
			expectedID:               "10.0.0.1:test-interface",
			expectedIndex:            0,
			expectedIndexError:       "strconv.ParseInt: parsing \"iamnotanindex\": invalid syntax",
			expectedSpeed:            1000,
			expectedOperStatus:       devicemetadata.OperStatusUp,
			expectedAdminStatus:      devicemetadata.AdminStatusDown,
			expectedMetadata:         devicemetadata.InterfaceMetadata{},
			expectedInterfaceError:   "strconv.ParseInt: parsing \"iamnotanindex\": invalid syntax",
			expectedIPV4Address:      nil,
			expectedIPV4AddressError: "strconv.ParseInt: parsing \"iamnotanindex\": invalid syntax",
			expectedIPV6Address:      nil,
			expectedIPV6AddressError: "strconv.ParseInt: parsing \"iamnotanindex\": invalid syntax",
		},
		{
			name:      "invalid ip address",
			namespace: "test-ns",
			itf: client.CEdgeInterfaceState{
				VmanageSystemIP: "10.0.0.1",
				Ifname:          "test-interface",
				Ifindex:         "10",
				SpeedMbps:       "1000",
				IfOperStatus:    "if-oper-state-ready",
				IfAdminStatus:   "if-state-down",
				Description:     "Description",
				Hwaddr:          "00:01:02:03",
				IPAddress:       "hello",
				Ipv4SubnetMask:  "255.255.255.0",
				IPV6Address:     "hello2",
				InterfaceType:   "iana-iftype-ethernet-csmacd",
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
				Type:        6,
				IsPhysical:  boolPtr(true),
			},
			expectedIPV4Address:      nil,
			expectedIPV4AddressError: "invalid ip address",
			expectedIPV6Address:      nil,
			expectedIPV6AddressError: "invalid ip address",
		},
		{
			name:      "invalid mask",
			namespace: "test-ns",
			itf: client.CEdgeInterfaceState{
				VmanageSystemIP: "10.0.0.1",
				Ifname:          "test-interface",
				Ifindex:         "10",
				SpeedMbps:       "0.1",
				IfOperStatus:    "if-oper-state-ready",
				IfAdminStatus:   "if-state-down",
				Description:     "Description",
				Hwaddr:          "00:01:02:03",
				IPAddress:       "10.1.1.5",
				Ipv4SubnetMask:  "hellohello",
				InterfaceType:   "iana-iftype-ethernet-csmacd",
			},
			expectedID:          "10.0.0.1:test-interface",
			expectedIndex:       10,
			expectedSpeed:       0.1,
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
				Type:        6,
				IsPhysical:  boolPtr(true),
			},
			expectedIPV4Address:      nil,
			expectedIPV4AddressError: "invalid mask",
		},
		{
			name:      "unspecified ip address",
			namespace: "test-ns",
			itf: client.CEdgeInterfaceState{
				VmanageSystemIP: "10.0.0.1",
				Ifname:          "test-interface",
				Ifindex:         "10",
				SpeedMbps:       "0.1",
				IfOperStatus:    "if-oper-state-ready",
				IfAdminStatus:   "if-state-down",
				Description:     "Description",
				Hwaddr:          "00:01:02:03",
				IPAddress:       "0.0.0.0",
				Ipv4SubnetMask:  "255.255.255.0",
				InterfaceType:   "iana-iftype-ethernet-csmacd",
			},
			expectedID:          "10.0.0.1:test-interface",
			expectedIndex:       10,
			expectedSpeed:       0.1,
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
				Type:        6,
				IsPhysical:  boolPtr(true),
			},
			expectedIPV4Address:      nil,
			expectedIPV4AddressError: "invalid ip address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			itf := CEdgeInterface{tt.itf}

			index, err := itf.Index()
			if tt.expectedIndexError != "" {
				require.ErrorContains(t, err, tt.expectedIndexError)
			} else {
				require.NoError(t, err)
			}

			itfMetadata, err := itf.Metadata(tt.namespace)
			if tt.expectedInterfaceError != "" {
				require.ErrorContains(t, err, tt.expectedInterfaceError)
			} else {
				require.NoError(t, err)
			}

			ipv4Address, err := itf.IPV4AddressMetadata(tt.namespace)
			if tt.expectedIPV4AddressError != "" {
				require.ErrorContains(t, err, tt.expectedIPV4AddressError)
			} else {
				require.NoError(t, err)
			}

			ipv6Address, err := itf.IPV6AddressMetadata(tt.namespace)
			if tt.expectedIPV6AddressError != "" {
				require.ErrorContains(t, err, tt.expectedIPV6AddressError)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tt.expectedID, itf.ID())
			require.Equal(t, tt.expectedIndex, index)
			require.Equal(t, tt.expectedSpeed, itf.GetSpeedMbps())
			require.Equal(t, tt.expectedOperStatus, itf.OperStatus())
			require.Equal(t, tt.expectedAdminStatus, itf.AdminStatus())
			require.Equal(t, tt.expectedMetadata, itfMetadata)
			require.Equal(t, tt.expectedIPV4Address, ipv4Address)
			require.Equal(t, tt.expectedIPV6Address, ipv6Address)
		})
	}
}
