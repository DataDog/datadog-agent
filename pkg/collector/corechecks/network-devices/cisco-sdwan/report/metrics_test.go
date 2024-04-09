// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package report

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/client"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/payload"
)

type expectedMetric struct {
	method string
	name   string
	value  float64
	tags   []string
}

func TestSendDeviceMetrics(t *testing.T) {
	devices := []client.DeviceStatistics{
		{
			SystemIP:   "10.0.0.1",
			EntryTime:  10000,
			CPUUserNew: 30,
			CPUSystem:  10,
			MemUtil:    0.56,
			DiskUsed:   30,
			DiskAvail:  70,
		},
		{
			SystemIP:  "10.0.0.1",
			EntryTime: 11000,
			CPUUser:   10,
			CPUSystem: 10,
			MemUtil:   0.56,
			DiskUsed:  30,
			DiskAvail: 70,
		},
	}

	mockSender := mocksender.NewMockSender("foo")
	mockSender.On("GaugeWithTimestamp", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	sender := NewSDWanSender(mockSender, "my-ns")

	// Ensure device tags are correctly sent
	sender.SetDeviceTags(map[string][]string{
		"10.0.0.1": {
			"test:tag",
			"test2:tag2",
		},
	})

	sender.SendDeviceMetrics(devices)

	expectedTags := []string{
		"test:tag",
		"test2:tag2",
	}

	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", ciscoSDWANMetricPrefix+"cpu.usage", 30+10, "", expectedTags, 10)
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", ciscoSDWANMetricPrefix+"cpu.usage", 10+10, "", expectedTags, 11) // Assert we fallback to cpu_user
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", ciscoSDWANMetricPrefix+"memory.usage", float64(0.56)*100, "", expectedTags, 10)
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", ciscoSDWANMetricPrefix+"disk.usage", 30, "", expectedTags, 10)
	require.Equal(t, map[string]float64{
		"device_metrics:test:tag,test2:tag2": 11000,
	}, sender.lastTimeSent)

	devices = append(devices, client.DeviceStatistics{
		SystemIP:   "10.0.0.1",
		EntryTime:  15000,
		CPUUserNew: 20,
		CPUSystem:  0,
		MemUtil:    0.75,
		DiskUsed:   35,
		DiskAvail:  65,
	})

	mockSender.ResetCalls()

	sender.SendDeviceMetrics(devices)

	// Asset only 3 calls have been made : the first statistics
	// entry should be ignored as it has already been sent
	mockSender.AssertNumberOfCalls(t, "GaugeWithTimestamp", 3)

	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", ciscoSDWANMetricPrefix+"cpu.usage", 20+0, "", expectedTags, 15)
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", ciscoSDWANMetricPrefix+"memory.usage", float64(0.75)*100, "", expectedTags, 15)
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", ciscoSDWANMetricPrefix+"disk.usage", 35, "", expectedTags, 15)
	require.Equal(t, map[string]float64{
		"device_metrics:test:tag,test2:tag2": 15000,
	}, sender.lastTimeSent)
}

func TestSendInterfaceMetrics(t *testing.T) {
	interfaceStats := []client.InterfaceStats{
		{
			VmanageSystemIP:        "10.0.0.1",
			EntryTime:              10000,
			VpnID:                  10,
			Interface:              "interface-1",
			TxOctets:               10,
			RxOctets:               20,
			TxKbps:                 250,
			RxKbps:                 13,
			TxPkts:                 500,
			RxPkts:                 100,
			DownCapacityPercentage: 12,
			UpCapacityPercentage:   1,
			RxErrors:               0,
			TxErrors:               17,
			RxDrops:                65,
			TxDrops:                2,
		},
	}

	interfaceMap := map[string]payload.CiscoInterface{
		"10.0.0.1:interface-1": &payload.VEdgeInterface{
			InterfaceState: client.InterfaceState{
				Ifindex:       10,
				SpeedMbps:     "500",
				IfOperStatus:  "Up",
				IfAdminStatus: "Up",
			},
		},
	}

	mockSender := mocksender.NewMockSender("foo")
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("GaugeWithTimestamp", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("CountWithTimestamp", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	sender := NewSDWanSender(mockSender, "my-ns")

	// Ensure device tags are correctly sent
	sender.SetDeviceTags(map[string][]string{
		"10.0.0.1": {
			"test:tag",
			"test2:tag2",
		},
	})

	sender.SendInterfaceMetrics(interfaceStats, interfaceMap)

	expectedTags := []string{
		"test:tag",
		"test2:tag2",
		"interface:interface-1",
		"interface_index:10",
	}

	mockSender.AssertMetricWithTimestamp(t, "CountWithTimestamp", ciscoSDWANMetricPrefix+"interface.rx_bits", 20*8, "", expectedTags, 10)
	mockSender.AssertMetricWithTimestamp(t, "CountWithTimestamp", ciscoSDWANMetricPrefix+"interface.tx_bits", 10*8, "", expectedTags, 10)
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", ciscoSDWANMetricPrefix+"interface.rx_kbps", 13, "", expectedTags, 10)
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", ciscoSDWANMetricPrefix+"interface.tx_kbps", 250, "", expectedTags, 10)
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", ciscoSDWANMetricPrefix+"interface.rx_bandwidth_usage", 12, "", expectedTags, 10)
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", ciscoSDWANMetricPrefix+"interface.tx_bandwidth_usage", 1, "", expectedTags, 10)
	mockSender.AssertMetricWithTimestamp(t, "CountWithTimestamp", ciscoSDWANMetricPrefix+"interface.rx_errors", 0, "", expectedTags, 10)
	mockSender.AssertMetricWithTimestamp(t, "CountWithTimestamp", ciscoSDWANMetricPrefix+"interface.tx_errors", 17, "", expectedTags, 10)
	mockSender.AssertMetricWithTimestamp(t, "CountWithTimestamp", ciscoSDWANMetricPrefix+"interface.rx_drops", 65, "", expectedTags, 10)
	mockSender.AssertMetricWithTimestamp(t, "CountWithTimestamp", ciscoSDWANMetricPrefix+"interface.tx_drops", 2, "", expectedTags, 10)
	require.Equal(t, map[string]float64{
		"interface_metrics:test:tag,test2:tag2,interface:interface-1,vpn_id:10,interface_index:10": 10000,
	}, sender.lastTimeSent)

	mockSender.ResetCalls()

	sender.SendInterfaceMetrics(interfaceStats, interfaceMap)

	// Assert metrics have not been re-sent and last time sent has been updated
	mockSender.AssertNumberOfCalls(t, "GaugeWithTimestamp", 0)
	mockSender.AssertNumberOfCalls(t, "CountWithTimestamp", 0)
	require.Equal(t, map[string]float64{
		"interface_metrics:test:tag,test2:tag2,interface:interface-1,vpn_id:10,interface_index:10": 10000,
	}, sender.lastTimeSent)
}

func TestSendDeviceUptimeMetrics(t *testing.T) {
	tests := []struct {
		name           string
		uptimes        map[string]float64
		tags           map[string][]string
		expectedMetric []expectedMetric
	}{
		{
			name: "Report device uptime",
			uptimes: map[string]float64{
				"10.0.0.1": 100000,
			},
			tags: map[string][]string{
				"10.0.0.1": {
					"device_name:10.0.0.1",
					"device_namespace:cisco-sdwan",
					"device_vendor:cisco",
					"hostname:test-device",
					"system_ip:10.0.0.1",
					"site_id:100",
				},
			},
			expectedMetric: []expectedMetric{
				{
					method: "Gauge",
					name:   ciscoSDWANMetricPrefix + "device.uptime",
					value:  100000,
					tags: []string{
						"device_name:10.0.0.1",
						"device_namespace:cisco-sdwan",
						"device_vendor:cisco",
						"hostname:test-device",
						"system_ip:10.0.0.1",
						"site_id:100",
					},
				},
			},
		},
		{
			name: "Report devices uptimes",
			uptimes: map[string]float64{
				"10.0.0.1": 100000,
				"10.0.0.2": 100,
			},
			tags: map[string][]string{
				"10.0.0.1": {
					"device_name:10.0.0.1",
					"device_namespace:cisco-sdwan",
					"device_vendor:cisco",
					"hostname:test-device",
					"system_ip:10.0.0.1",
					"site_id:100",
				},
				"10.0.0.2": {
					"device_name:10.0.0.2",
					"device_namespace:cisco-sdwan",
					"device_vendor:cisco",
					"hostname:test-device",
					"system_ip:10.0.0.2",
					"site_id:102",
				},
			},
			expectedMetric: []expectedMetric{
				{
					method: "Gauge",
					name:   ciscoSDWANMetricPrefix + "device.uptime",
					value:  100000,
					tags: []string{
						"device_name:10.0.0.1",
						"device_namespace:cisco-sdwan",
						"device_vendor:cisco",
						"hostname:test-device",
						"system_ip:10.0.0.1",
						"site_id:100",
					},
				},
				{
					method: "Gauge",
					name:   ciscoSDWANMetricPrefix + "device.uptime",
					value:  100,
					tags: []string{
						"device_name:10.0.0.2",
						"device_namespace:cisco-sdwan",
						"device_vendor:cisco",
						"hostname:test-device",
						"system_ip:10.0.0.2",
						"site_id:102",
					},
				},
			},
		},
		{
			name: "Empty tags",
			uptimes: map[string]float64{
				"10.0.0.1": 100000,
			},
			tags: map[string][]string{},
			expectedMetric: []expectedMetric{
				{
					method: "Gauge",
					name:   ciscoSDWANMetricPrefix + "device.uptime",
					value:  100000,
					tags: []string{
						"system_ip:10.0.0.1",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := mocksender.NewMockSender("foo")
			mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			sender := NewSDWanSender(mockSender, "my-ns")
			sender.SetDeviceTags(tt.tags)
			sender.SendUptimeMetrics(tt.uptimes)

			for _, metric := range tt.expectedMetric {
				mockSender.AssertMetric(t, metric.method, metric.name, metric.value, "", metric.tags)
			}
		})
	}
}

func TestSendApplicationAwareRoutingMetrics(t *testing.T) {
	appStats := []client.AppRouteStatistics{
		{
			VmanageSystemIP: "10.0.0.1",
			RemoteSystemIP:  "10.0.0.2",
			EntryTime:       10000,
			LocalColor:      "mpls",
			RemoteColor:     "public-internet",
			State:           "Up",
			TxOctets:        10,
			RxOctets:        20,
			Latency:         13,
			Jitter:          2.1,
			LossPercentage:  0.01,
			VqoeScore:       8,
			RxPkts:          512,
			TxPkts:          203,
		},
	}

	mockSender := mocksender.NewMockSender("foo")
	mockSender.On("GaugeWithTimestamp", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("CountWithTimestamp", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	sender := NewSDWanSender(mockSender, "my-ns")

	// Ensure device tags are correctly sent
	sender.SetDeviceTags(map[string][]string{
		"10.0.0.1": {
			"test:tag",
			"test2:tag2",
		},
		"10.0.0.2": {
			"test3:tag3",
			"test4:tag4",
		},
	})

	sender.SendAppRouteMetrics(appStats)

	expectedTags := []string{
		"test:tag",
		"test2:tag2",
		"remote_test3:tag3",
		"remote_test4:tag4",
		"local_color:mpls",
		"remote_color:public-internet",
		"state:Up",
	}

	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", ciscoSDWANMetricPrefix+"tunnel.status", 1, "", expectedTags, 10)
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", ciscoSDWANMetricPrefix+"tunnel.latency", 13, "", expectedTags, 10)
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", ciscoSDWANMetricPrefix+"tunnel.jitter", 2.1, "", expectedTags, 10)
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", ciscoSDWANMetricPrefix+"tunnel.loss", 0.01, "", expectedTags, 10)
	mockSender.AssertMetricWithTimestamp(t, "GaugeWithTimestamp", ciscoSDWANMetricPrefix+"tunnel.qoe", 8, "", expectedTags, 10)
	mockSender.AssertMetricWithTimestamp(t, "CountWithTimestamp", ciscoSDWANMetricPrefix+"tunnel.rx_bits", 20*8, "", expectedTags, 10)
	mockSender.AssertMetricWithTimestamp(t, "CountWithTimestamp", ciscoSDWANMetricPrefix+"tunnel.tx_bits", 10*8, "", expectedTags, 10)
	mockSender.AssertMetricWithTimestamp(t, "CountWithTimestamp", ciscoSDWANMetricPrefix+"tunnel.rx_packets", 512, "", expectedTags, 10)
	mockSender.AssertMetricWithTimestamp(t, "CountWithTimestamp", ciscoSDWANMetricPrefix+"tunnel.tx_packets", 203, "", expectedTags, 10)
	require.Equal(t, map[string]float64{
		"tunnel_metrics:test:tag,test2:tag2,remote_test3:tag3,remote_test4:tag4,local_color:mpls,remote_color:public-internet,state:Up": 10000,
	}, sender.lastTimeSent)

	mockSender.ResetCalls()

	sender.SendAppRouteMetrics(appStats)

	// Assert metrics have not been re-sent
	mockSender.AssertNumberOfCalls(t, "GaugeWithTimestamp", 0)
	mockSender.AssertNumberOfCalls(t, "CountWithTimestamp", 0)
	require.Equal(t, map[string]float64{
		"tunnel_metrics:test:tag,test2:tag2,remote_test3:tag3,remote_test4:tag4,local_color:mpls,remote_color:public-internet,state:Up": 10000,
	}, sender.lastTimeSent)
}

func TestSendControlConnectionMetrics(t *testing.T) {
	tests := []struct {
		name               string
		controlConnections []client.ControlConnections
		tags               map[string][]string
		expectedMetric     []expectedMetric
	}{
		{
			name: "Report device control connections",
			controlConnections: []client.ControlConnections{
				{
					VmanageSystemIP: "10.0.0.1",
					SystemIP:        "10.0.0.2",
					LocalColor:      "public-internet",
					RemoteColor:     "mpls",
					PeerType:        "vmanage",
					State:           "up",
					PrivateIP:       "10.1.1.11",
				},
				{
					VmanageSystemIP: "10.0.0.1",
					SystemIP:        "10.0.0.3",
					LocalColor:      "public-internet",
					RemoteColor:     "mpls",
					PeerType:        "vbond",
					State:           "down",
					PrivateIP:       "10.1.1.13",
				},
			},
			tags: map[string][]string{
				"10.0.0.1": {
					"device_name:10.0.0.1",
					"device_namespace:cisco-sdwan",
					"device_vendor:cisco",
					"hostname:test-device",
					"system_ip:10.0.0.1",
					"site_id:100",
				},
				"10.0.0.2": {
					"device_name:10.0.0.2",
					"device_namespace:cisco-sdwan",
					"device_vendor:cisco",
					"hostname:test-vmanage",
					"system_ip:10.0.0.2",
					"site_id:102",
				},
				"10.0.0.3": {
					"device_name:10.0.0.3",
					"device_namespace:cisco-sdwan",
					"device_vendor:cisco",
					"hostname:test-vbond",
					"system_ip:10.0.0.3",
					"site_id:102",
				},
			},
			expectedMetric: []expectedMetric{
				{
					method: "Gauge",
					value:  1,
					tags: []string{
						"device_name:10.0.0.1",
						"device_namespace:cisco-sdwan",
						"device_vendor:cisco",
						"hostname:test-device",
						"system_ip:10.0.0.1",
						"site_id:100",
						"remote_device_name:10.0.0.2",
						"remote_device_vendor:cisco",
						"remote_hostname:test-vmanage",
						"remote_system_ip:10.0.0.2",
						"remote_site_id:102",
						"private_ip:10.1.1.11",
						"local_color:public-internet",
						"remote_color:mpls",
						"peer_type:vmanage",
						"state:up",
					},
				},
				{
					method: "Gauge",
					value:  0,
					tags: []string{
						"device_name:10.0.0.1",
						"device_namespace:cisco-sdwan",
						"device_vendor:cisco",
						"hostname:test-device",
						"system_ip:10.0.0.1",
						"site_id:100",
						"remote_device_name:10.0.0.3",
						"remote_device_vendor:cisco",
						"remote_hostname:test-vbond",
						"remote_system_ip:10.0.0.3",
						"remote_site_id:102",
						"private_ip:10.1.1.13",
						"local_color:public-internet",
						"remote_color:mpls",
						"peer_type:vbond",
						"state:down",
					},
				},
			},
		},
		{
			name: "Missing remote tags still sets remote system ip tag",
			controlConnections: []client.ControlConnections{
				{
					VmanageSystemIP: "10.0.0.1",
					SystemIP:        "10.0.0.2",
					LocalColor:      "public-internet",
					RemoteColor:     "mpls",
					PeerType:        "vmanage",
					State:           "up",
					PrivateIP:       "10.1.1.11",
				},
			},
			tags: map[string][]string{
				"10.0.0.1": {
					"device_name:10.0.0.1",
					"device_namespace:cisco-sdwan",
					"device_vendor:cisco",
					"hostname:test-device",
					"system_ip:10.0.0.1",
					"site_id:100",
				},
			},
			expectedMetric: []expectedMetric{
				{
					method: "Gauge",
					value:  1,
					tags: []string{
						"device_name:10.0.0.1",
						"device_namespace:cisco-sdwan",
						"device_vendor:cisco",
						"hostname:test-device",
						"system_ip:10.0.0.1",
						"site_id:100",
						"remote_system_ip:10.0.0.2",
						"private_ip:10.1.1.11",
						"local_color:public-internet",
						"remote_color:mpls",
						"peer_type:vmanage",
						"state:up",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := mocksender.NewMockSender("foo")
			mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			sender := NewSDWanSender(mockSender, "my-ns")
			sender.SetDeviceTags(tt.tags)
			sender.SendControlConnectionMetrics(tt.controlConnections)

			for _, metric := range tt.expectedMetric {
				mockSender.AssertMetric(t, metric.method, ciscoSDWANMetricPrefix+"control_connection.status", metric.value, "", metric.tags)
			}
		})
	}
}

func TestSendOMPPeerMetrics(t *testing.T) {
	tests := []struct {
		name           string
		ompPeers       []client.OMPPeer
		tags           map[string][]string
		expectedMetric []expectedMetric
	}{
		{
			name: "Report device omp peers",
			ompPeers: []client.OMPPeer{
				{
					VmanageSystemIP: "10.0.0.1",
					Peer:            "10.0.0.2",
					Legit:           "yes",
					Refresh:         "supported",
					Type:            "vsmart",
					State:           "up",
				},
				{
					VmanageSystemIP: "10.0.0.1",
					Peer:            "10.0.0.3",
					Legit:           "yes",
					Refresh:         "unsupported",
					Type:            "vedge",
					State:           "down",
				},
			},
			tags: map[string][]string{
				"10.0.0.1": {
					"device_name:10.0.0.1",
					"device_namespace:cisco-sdwan",
					"device_vendor:cisco",
					"hostname:test-device",
					"system_ip:10.0.0.1",
					"site_id:100",
				},
				"10.0.0.2": {
					"device_name:10.0.0.2",
					"device_namespace:cisco-sdwan",
					"device_vendor:cisco",
					"hostname:test-vsmart",
					"system_ip:10.0.0.2",
					"site_id:102",
				},
				"10.0.0.3": {
					"device_name:10.0.0.3",
					"device_namespace:cisco-sdwan",
					"device_vendor:cisco",
					"hostname:test-device2",
					"system_ip:10.0.0.3",
					"site_id:110",
				},
			},
			expectedMetric: []expectedMetric{
				{
					method: "Gauge",
					value:  1,
					tags: []string{
						"device_name:10.0.0.1",
						"device_namespace:cisco-sdwan",
						"device_vendor:cisco",
						"hostname:test-device",
						"system_ip:10.0.0.1",
						"site_id:100",
						"remote_device_name:10.0.0.2",
						"remote_device_vendor:cisco",
						"remote_hostname:test-vsmart",
						"remote_system_ip:10.0.0.2",
						"remote_site_id:102",
						"legit:yes",
						"refresh:supported",
						"type:vsmart",
						"state:up",
					},
				},
				{
					method: "Gauge",
					value:  0,
					tags: []string{
						"device_name:10.0.0.1",
						"device_namespace:cisco-sdwan",
						"device_vendor:cisco",
						"hostname:test-device",
						"system_ip:10.0.0.1",
						"site_id:100",
						"remote_device_name:10.0.0.3",
						"remote_device_vendor:cisco",
						"remote_hostname:test-device2",
						"remote_system_ip:10.0.0.3",
						"remote_site_id:110",
						"legit:yes",
						"refresh:unsupported",
						"type:vedge",
						"state:down",
					},
				},
			},
		},
		{
			name: "Missing remote tags still sets remote system ip tag",
			ompPeers: []client.OMPPeer{
				{
					VmanageSystemIP: "10.0.0.1",
					Peer:            "10.0.0.2",
					Legit:           "yes",
					Refresh:         "supported",
					Type:            "vsmart",
					State:           "up",
				},
			},
			tags: map[string][]string{
				"10.0.0.1": {
					"device_name:10.0.0.1",
					"device_namespace:cisco-sdwan",
					"device_vendor:cisco",
					"hostname:test-device",
					"system_ip:10.0.0.1",
					"site_id:100",
				},
			},
			expectedMetric: []expectedMetric{
				{
					method: "Gauge",
					value:  1,
					tags: []string{
						"device_name:10.0.0.1",
						"device_namespace:cisco-sdwan",
						"device_vendor:cisco",
						"hostname:test-device",
						"system_ip:10.0.0.1",
						"site_id:100",
						"remote_system_ip:10.0.0.2",
						"legit:yes",
						"refresh:supported",
						"type:vsmart",
						"state:up",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := mocksender.NewMockSender("foo")
			mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			sender := NewSDWanSender(mockSender, "my-ns")
			sender.SetDeviceTags(tt.tags)
			sender.SendOMPPeerMetrics(tt.ompPeers)

			for _, metric := range tt.expectedMetric {
				mockSender.AssertMetric(t, metric.method, ciscoSDWANMetricPrefix+"omp_peer.status", metric.value, "", metric.tags)
			}
		})
	}
}

func TestSendBFDSessionMetrics(t *testing.T) {
	tests := []struct {
		name           string
		bfdSessions    []client.BFDSession
		tags           map[string][]string
		expectedMetric []expectedMetric
	}{
		{
			name: "Report device omp peers",
			bfdSessions: []client.BFDSession{
				{
					VmanageSystemIP: "10.0.0.1",
					SystemIP:        "10.0.0.2",
					LocalColor:      "mpls",
					Color:           "public-internet",
					Proto:           "ipsec",
					State:           "up",
				},
				{
					VmanageSystemIP: "10.0.0.1",
					SystemIP:        "10.0.0.3",
					LocalColor:      "lte",
					Color:           "mpls",
					Proto:           "ipsec",
					State:           "down",
				},
			},
			tags: map[string][]string{
				"10.0.0.1": {
					"device_name:10.0.0.1",
					"device_namespace:cisco-sdwan",
					"device_vendor:cisco",
					"hostname:test-device",
					"system_ip:10.0.0.1",
					"site_id:100",
				},
				"10.0.0.2": {
					"device_name:10.0.0.2",
					"device_namespace:cisco-sdwan",
					"device_vendor:cisco",
					"hostname:test-vsmart",
					"system_ip:10.0.0.2",
					"site_id:102",
				},
				"10.0.0.3": {
					"device_name:10.0.0.3",
					"device_namespace:cisco-sdwan",
					"device_vendor:cisco",
					"hostname:test-device2",
					"system_ip:10.0.0.3",
					"site_id:110",
				},
			},
			expectedMetric: []expectedMetric{
				{
					method: "Gauge",
					value:  1,
					tags: []string{
						"device_name:10.0.0.1",
						"device_namespace:cisco-sdwan",
						"device_vendor:cisco",
						"hostname:test-device",
						"system_ip:10.0.0.1",
						"site_id:100",
						"remote_device_name:10.0.0.2",
						"remote_device_vendor:cisco",
						"remote_hostname:test-vsmart",
						"remote_system_ip:10.0.0.2",
						"remote_site_id:102",
						"local_color:mpls",
						"remote_color:public-internet",
						"proto:ipsec",
						"state:up",
					},
				},
				{
					method: "Gauge",
					value:  0,
					tags: []string{
						"device_name:10.0.0.1",
						"device_namespace:cisco-sdwan",
						"device_vendor:cisco",
						"hostname:test-device",
						"system_ip:10.0.0.1",
						"site_id:100",
						"remote_device_name:10.0.0.3",
						"remote_device_vendor:cisco",
						"remote_hostname:test-device2",
						"remote_system_ip:10.0.0.3",
						"remote_site_id:110",
						"local_color:lte",
						"remote_color:mpls",
						"proto:ipsec",
						"state:down",
					},
				},
			},
		},
		{
			name: "Missing remote tags still sets remote system ip tag",
			bfdSessions: []client.BFDSession{
				{
					VmanageSystemIP: "10.0.0.1",
					SystemIP:        "10.0.0.2",
					LocalColor:      "mpls",
					Color:           "public-internet",
					Proto:           "ipsec",
					State:           "up",
				},
			},
			tags: map[string][]string{
				"10.0.0.1": {
					"device_name:10.0.0.1",
					"device_namespace:cisco-sdwan",
					"device_vendor:cisco",
					"hostname:test-device",
					"system_ip:10.0.0.1",
					"site_id:100",
				},
			},
			expectedMetric: []expectedMetric{
				{
					method: "Gauge",
					value:  1,
					tags: []string{
						"device_name:10.0.0.1",
						"device_namespace:cisco-sdwan",
						"device_vendor:cisco",
						"hostname:test-device",
						"system_ip:10.0.0.1",
						"site_id:100",
						"remote_system_ip:10.0.0.2",
						"local_color:mpls",
						"remote_color:public-internet",
						"proto:ipsec",
						"state:up",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := mocksender.NewMockSender("foo")
			mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			sender := NewSDWanSender(mockSender, "my-ns")
			sender.SetDeviceTags(tt.tags)
			sender.SendBFDSessionMetrics(tt.bfdSessions)

			for _, metric := range tt.expectedMetric {
				mockSender.AssertMetric(t, metric.method, ciscoSDWANMetricPrefix+"bfd_session.status", metric.value, "", metric.tags)
			}
		})
	}
}

func TestSendDeviceCountersMetrics(t *testing.T) {
	tests := []struct {
		name           string
		deviceCounters []client.DeviceCounters
		tags           map[string][]string
		expectedMetric []expectedMetric
	}{
		{
			name: "Report device counters",
			deviceCounters: []client.DeviceCounters{
				{
					SystemIP:    "10.0.0.1",
					RebootCount: 10,
					CrashCount:  1,
				},
				{
					SystemIP:    "10.0.0.2",
					RebootCount: 1,
					CrashCount:  0,
				},
			},
			tags: map[string][]string{
				"10.0.0.1": {
					"device_name:10.0.0.1",
					"device_namespace:cisco-sdwan",
					"device_vendor:cisco",
					"hostname:test-device",
					"system_ip:10.0.0.1",
					"site_id:100",
				},
				"10.0.0.2": {
					"device_name:10.0.0.2",
					"device_namespace:cisco-sdwan",
					"device_vendor:cisco",
					"hostname:test-vsmart",
					"system_ip:10.0.0.2",
					"site_id:102",
				},
			},
			expectedMetric: []expectedMetric{
				{
					method: "MonotonicCount",
					value:  10,
					name:   ciscoSDWANMetricPrefix + "reboot.count",
					tags: []string{
						"device_name:10.0.0.1",
						"device_namespace:cisco-sdwan",
						"device_vendor:cisco",
						"hostname:test-device",
						"system_ip:10.0.0.1",
						"site_id:100",
					},
				},
				{
					method: "MonotonicCount",
					value:  1,
					name:   ciscoSDWANMetricPrefix + "crash.count",
					tags: []string{
						"device_name:10.0.0.1",
						"device_namespace:cisco-sdwan",
						"device_vendor:cisco",
						"hostname:test-device",
						"system_ip:10.0.0.1",
						"site_id:100",
					},
				},
				{
					method: "MonotonicCount",
					value:  1,
					name:   ciscoSDWANMetricPrefix + "reboot.count",
					tags: []string{
						"device_name:10.0.0.2",
						"device_namespace:cisco-sdwan",
						"device_vendor:cisco",
						"hostname:test-vsmart",
						"system_ip:10.0.0.2",
						"site_id:102",
					},
				},
				{
					method: "MonotonicCount",
					value:  0,
					name:   ciscoSDWANMetricPrefix + "crash.count",
					tags: []string{
						"device_name:10.0.0.2",
						"device_namespace:cisco-sdwan",
						"device_vendor:cisco",
						"hostname:test-vsmart",
						"system_ip:10.0.0.2",
						"site_id:102",
					},
				},
			},
		},
		{
			name: "Missing device tags still sets system ip tag",
			deviceCounters: []client.DeviceCounters{
				{
					SystemIP:    "10.0.0.1",
					RebootCount: 10,
					CrashCount:  1,
				},
			},
			tags: map[string][]string{},
			expectedMetric: []expectedMetric{
				{
					method: "MonotonicCount",
					value:  10,
					name:   ciscoSDWANMetricPrefix + "reboot.count",
					tags: []string{
						"system_ip:10.0.0.1",
					},
				},
				{
					method: "MonotonicCount",
					value:  1,
					name:   ciscoSDWANMetricPrefix + "crash.count",
					tags: []string{
						"system_ip:10.0.0.1",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := mocksender.NewMockSender("foo")
			mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			sender := NewSDWanSender(mockSender, "my-ns")
			sender.SetDeviceTags(tt.tags)
			sender.SendDeviceCountersMetrics(tt.deviceCounters)

			for _, metric := range tt.expectedMetric {
				mockSender.AssertMetric(t, metric.method, metric.name, metric.value, "", metric.tags)
			}
		})
	}
}

func TestTimestampExpiration(t *testing.T) {
	TimeNow = mockTimeNow
	ms := NewSDWanSender(nil, "test-ns")

	testTimestamps := map[string]float64{
		"test-id":   1000,
		"test-id-2": 946684700,
	}

	ms.updateTimestamps(testTimestamps)
	ms.expireTimeSent()

	// Assert "test-id" is expired
	require.Equal(t, map[string]float64{
		"test-id-2": 946684700,
	}, ms.lastTimeSent)
}
