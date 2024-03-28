// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package fixtures contains example responses from Cisco SD-WAN API
package fixtures

// FakePayload generates a fake API response
func FakePayload(content string) string {
	return `
{
    "header": {
        "generatedOn": 1709049190625,
        "viewKeys": {
            "uniqueKey": [
                "system-ip"
            ],
            "preferenceKey": "grid-Device"
        },
        "columns": [

        ],
        "fields": [
            {
                "property": "host-name",
                "dataType": "string",
                "display": "iconAndText"
            }
        ]
    },
	"data":
` + content + `
}
`
}

// GetDevices /dataservice/device
const GetDevices = `
[
	{
		"deviceId": "10.10.1.1",
		"system-ip": "10.10.1.1",
		"host-name": "Manager",
		"reachability": "reachable",
		"status": "normal",
		"personality": "vmanage",
		"device-type": "vmanage",
		"timezone": "UTC",
		"device-groups": [
			"No groups"
		],
		"lastupdated": 1708941610464,
		"domain-id": "0",
		"board-serial": "61FA4073B0169C46F4F498B8CA2C5C7A4A5510F9",
		"certificate-validity": "Valid",
		"max-controllers": "0",
		"uuid": "dfa48eb3-9e5b-4bca-a25a-2aace87fdf62",
		"controlConnections": "8",
		"device-model": "vmanage",
		"version": "20.12.1",
		"connectedVManages": [
			"10.10.1.1"
		],
		"site-id": "101",
		"latitude": "37.666684",
		"longitude": "-122.777023",
		"isDeviceGeoData": false,
		"platform": "x86_64",
		"uptime-date": 1708939320000,
		"statusOrder": 4,
		"device-os": "next",
		"validity": "valid",
		"state": "green",
		"state_description": "All daemons up",
		"model_sku": "None",
		"local-system-ip": "10.10.1.1",
		"total_cpu_count": "4",
		"testbed_mode": false,
		"layoutLevel": 1,
		"site-name": "SITE_101"
	}
]
`

// GetDevicesCounters /dataservice/device/counters
const GetDevicesCounters = `
[
	{
		"system-ip": "10.10.1.12",
		"number-vsmart-control-connections": 1,
		"expectedControlConnections": 1,
		"isMTEdge": false,
		"rebootCount": 3,
		"crashCount": 0
	}
]
`

// GetVEdgeInterfaces /dataservice/device/state/interface
const GetVEdgeInterfaces = `
[
	{
		"recordId": "0:InterfaceNode:1695315985274:3156",
		"ifindex": 3,
		"vdevice-name": "10.10.1.5",
		"if-admin-status": "Up",
		"createTimeStamp": 1695315985274,
		"duplex": "full",
		"vpn-id": "0",
		"vdevice-host-name": "Controller",
		"ipv6-address": "-",
		"ip-address": "-",
		"speed-mbps": "1000",
		"vdevice-dataKey": "10.10.1.5-0-system-ipv6",
		"@rid": 3128,
		"ifname": "system",
		"vmanage-system-ip": "10.10.1.5",
		"af-type": "ipv6",
		"lastupdated": 1699342559908,
		"port-type": "loopback",
		"if-oper-status": "Up",
		"auto-neg": "false",
		"encap-type": "null"
	}
]
`

// GetCEdgeInterfaces /dataservice/data/device/state/CEdgeInterface
const GetCEdgeInterfaces = `
[
	{
		"recordId": "0:CEdgeInterfaceNode:1707919153393:231",
		"vdevice-name": "10.10.1.17",
		"rx-errors": 0,
		"if-admin-status": "if-state-up",
		"ipv6-tcp-adjust-mss": "0",
		"tx-errors": 0,
		"@rid": 3728,
		"ifname": "GigabitEthernet4",
		"interface-type": "iana-iftype-ethernet-csmacd",
		"if-oper-status": "if-oper-state-ready",
		"ifindex": "4",
		"ipv4-tcp-adjust-mss": "0",
		"rx-packets": 2931,
		"bia-address": "52:54:00:0b:6e:90",
		"createTimeStamp": 1707919153393,
		"vpn-id": "0",
		"vdevice-host-name": "Site3-cEdge01",
		"ipv4-subnet-mask": "0.0.0.0",
		"tx-drops": 0,
		"mtu": "1500",
		"rx-drops": 0,
		"ip-address": "0.0.0.0",
		"speed-mbps": "1000",
		"hwaddr": "52:54:00:0b:6e:90",
		"vdevice-dataKey": "10.10.1.17-0-GigabitEthernet4-0.0.0.0-52:54:00:0b:6e:90",
		"auto-downstream-bandwidth": "0",
		"vmanage-system-ip": "10.10.1.17",
		"tx-octets": 1010850,
		"auto-upstream-bandwidth": "0",
		"tx-packets": 2930,
		"rx-octets": 1011195,
		"lastupdated": 1709027106590
	}
]
`

// GetInterfacesMetrics /dataservice/data/device/statistics/interfacestatistics
const GetInterfacesMetrics = `
[
	{
		"down_capacity_percentage": 0,
		"tx_pps": 2,
		"total_mbps": 0,
		"device_model": "vedge-C8000V-SD-ROUTING",
		"rx_kbps": 10.4,
		"interface": "GigabitEthernet3",
		"tx_octets": 4,
		"oper_status": "Down",
		"rx_errors": 2,
		"bw_down": 1000000,
		"tx_pkts": 10,
		"tx_errors": 506,
		"rx_octets": 23,
		"statcycletime": 1709050200011,
		"admin_status": "Down",
		"bw_up": 1000000,
		"interface_type": "physical",
		"tenant": "default",
		"entry_time": 1709049697985,
		"rx_pkts": 12,
		"af_type": "None",
		"rx_pps": 67.3,
		"vmanage_system_ip": "10.10.1.22",
		"tx_drops": 3,
		"rx_drops": 6,
		"tx_kbps": 9.8,
		"vdevice_name": "10.10.1.22",
		"up_capacity_percentage": 0.8,
		"vip_idx": 7,
		"host_name": "Site12-Edge01",
		"vpn_id": 0,
		"id": "G2ZU640BLWo2CjvLf13M"
	}
]
`

// GetDeviceHardwareStatistics /dataservice/data/device/statistics/devicesystemstatusstatistics
const GetDeviceHardwareStatistics = `
[
	{
		"mem_used": 597504000,
		"disk_avail": 7245897728,
		"device_model": "vsmart",
		"mem_cached": 357904384,
		"mem_util": 0.15,
		"min1_avg": 0.08,
		"disk_used": 293187584,
		"statcycletime": 1709050800019,
		"tenant": "default",
		"entry_time": 1709050342874,
		"runningp": 0,
		"cpu_user": 0.29,
		"cpu_idle_new": 99.3,
		"vip_time": 1709050342874,
		"min15_avg": 0.01,
		"totalp": 240,
		"cpu_idle": 99.3,
		"mem_buffers": 104796160,
		"cpu_system": 0.41,
		"vmanage_system_ip": "10.10.1.5",
		"min5_avg": 0.04,
		"cpu_min1_avg": 0.04,
		"mem_free": 2940260352,
		"vdevice_name": "10.10.1.5",
		"vip_idx": 1850,
		"cpu_min15_avg": 0.005,
		"system_ip": "10.10.1.5",
		"cpu_user_new": 0.29,
		"cpu_system_new": 0.41,
		"host_name": "Controller",
		"cpu_min5_avg": 0.02,
		"id": "jWZd640BLWo2CjvLp16Y"
	}
]
`

// GetApplicationAwareRoutingMetrics /dataservice/data/device/statistics/approutestatsstatistics
const GetApplicationAwareRoutingMetrics = `
[
	{
		"remote_color": "public-internet",
		"fec_re": 0,
		"vqoe_score": 2,
		"device_model": "vedge-C8000V",
		"latency": 202,
		"tx_octets": 0,
		"dst_ip": "10.10.23.38",
		"local_color": "mpls",
		"src_ip": "10.10.23.10",
		"sla_class_names": "__all_tunnels__",
		"loss": 2,
		"total": 664,
		"tx_pkts": 0,
		"fec_tx": 0,
		"rx_octets": 0,
		"statcycletime": 1709050800047,
		"siteid": 1001,
		"state": "Up",
		"local_system_ip": "10.10.1.13",
		"tenant": "default",
		"entry_time": 1709050725125,
		"loss_percentage": 0.301,
		"app_probe_class": "None",
		"rx_pkts": 0,
		"vmanage_system_ip": "10.10.1.13",
		"fec_rx": 0,
		"src_port": 12346,
		"jitter": 0,
		"remote_system_ip": "10.10.1.11",
		"vdevice_name": "10.10.1.13",
		"proto": "IPSEC",
		"vip_idx": 2219,
		"dst_port": 12346,
		"name": "10.10.1.13:mpls-10.10.1.11:public-internet",
		"sla_class_list": "0",
		"tunnel_color": "mpls:public-internet",
		"host_name": "Site1-cEdge01",
		"id": "HWZd640BLWo2CjvLp1-y"
	}
]
`

// GetControlConnectionsState /dataservice/data/device/state/ControlConnection
const GetControlConnectionsState = `
[
	{
		"recordId": "0:ControlConnectionNode:1696947983842:12398",
		"domain-id": 0,
		"instance": 0,
		"vdevice-name": "10.10.1.1",
		"createTimeStamp": 1696947983842,
		"system-ip": "10.10.1.3",
		"remote-color": "default",
		"site-id": 0,
		"private-port": 12346,
		"vdevice-host-name": "Manager",
		"local-color": "default",
		"peer-type": "vbond",
		"v-org-name": "DevNet_SD-WAN_Sandbox",
		"protocol": "dtls",
		"vdevice-dataKey": "10.10.1.1-0-default-10.10.1.3-default",
		"@rid": 3308,
		"vmanage-system-ip": "10.10.1.1",
		"public-ip": "10.10.20.80",
		"public-port": 12346,
		"lastupdated": 1708940248121,
		"state": "up",
		"uptime-date": 1708939440000,
		"private-ip": "10.10.20.80"
	}
]
`

// GetOMPPeersState /dataservice/data/device/state/OMPPeer
const GetOMPPeersState = `
[
	{
		"recordId": "0:OMPPeerNode:1707968814231:648",
		"domain-id": 1,
		"vdevice-name": "10.10.1.5",
		"createTimeStamp": 1707968814231,
		"refresh": "supported",
		"site-id": 1001,
		"type": "vedge",
		"vdevice-host-name": "Controller",
		"vdevice-dataKey": "10.10.1.5-10.10.1.13-vedge",
		"@rid": 4183,
		"vmanage-system-ip": "10.10.1.5",
		"peer": "10.10.1.13",
		"legit": "yes",
		"region-id": "None",
		"lastupdated": 1707968814154,
		"state": "up",
		"filter-affinity-group-preference": "NA"
	}
]
`

// GetBFDSessionsState /dataservice/data/device/state/BFDSessions
const GetBFDSessionsState = `
[
	{
		"recordId": "0:BFDSessionsNode:1707919131595:204",
		"src-ip": "10.10.23.38",
		"dst-ip": "10.10.23.42",
		"vdevice-name": "10.10.1.11",
		"color": "public-internet",
		"src-port": 12346,
		"createTimeStamp": 1707919131595,
		"system-ip": "10.10.1.13",
		"dst-port": 12346,
		"site-id": 1001,
		"transitions": 0,
		"vdevice-host-name": "DC-cEdge01",
		"local-color": "public-internet",
		"detect-multiplier": "7",
		"vdevice-dataKey": "10.10.1.11-public-internet-10.10.1.13-public-internet-ipsec",
		"@rid": 3692,
		"vmanage-system-ip": "10.10.1.11",
		"proto": "ipsec",
		"lastupdated": 1708940180703,
		"state": "up",
		"tx-interval": 1000,
		"uptime-date": 1708939680000
	}
]
`
