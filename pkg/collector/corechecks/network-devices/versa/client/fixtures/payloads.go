// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package fixtures contains example responses from the Versa API
package fixtures

// GetOrganizations is a mock list of organizations
const GetOrganizations = `
{
    "totalCount": 2,
    "organizations": [
        {
            "uuid": "fakeUUID",
            "name": "datadog",
            "paraentOrg": "fakeParentOrg",
            "connectors": [
                "datadog-test",
                "datadog-other-test"
            ],
            "plan": "Default-All-Services-Plan",
            "globalOrgId": "418",
            "description": "DataDog Unit Test Fixture",
            "sharedControlPlane": true,
            "blockInterRegionRouting": true,
            "cpeDeploymentType": "SDWAN",
            "authType": "unitTest",
            "providerOrg": false,
            "depth": 10,
            "pushCaConfig": false
        },
		{
            "uuid": "fakeUUID2",
            "name": "datadog2",
            "paraentOrg": "fakeParentOrg2",
            "connectors": [
                "datadog-test",
                "datadog-other-test"
            ],
            "plan": "Default-All-Services-Plan",
            "globalOrgId": "418",
            "description": "DataDog Unit Test Fixture 2",
            "sharedControlPlane": false,
            "blockInterRegionRouting": false,
            "cpeDeploymentType": "SDWAN",
            "authType": "unitTest 2",
            "providerOrg": true,
            "depth": 10,
            "pushCaConfig": true
        }
    ]
}`

// GetChildAppliancesDetail retrieves a list of appliances with details
//
//nolint:misspell
const GetChildAppliancesDetail = `
[
    {
        "name": "branch-1",
        "uuid": "fakeUUID-branch-1",
        "applianceLocation": {
            "applianceName": "branch-1",
            "applianceUuid": "fakeUUID-branch-1",
            "locationId": "USA",
            "latitude": "0.00",
            "longitude": "0.00",
            "type": "branch"
        },
        "last-updated-time": "2025-04-24 20:26:11.0",
        "ping-status": "UNREACHABLE",
        "sync-status": "UNKNOWN",
        "yang-compatibility-status": "Unavailable",
        "services-status": "UNKNOWN",
        "overall-status": "NOT-APPLICABLE",
        "controll-status": "Unavailable",
        "path-status": "Unavailable",
        "intra-chassis-ha-status": {
            "ha-configured": false
        },
        "inter-chassis-ha-status": {
            "ha-configured": false
        },
        "templateStatus": "IN_SYNC",
        "ownerOrgUuid": "another-fakeUUID-branch-1",
        "ownerOrg": "datadog",
        "type": "branch",
        "sngCount": 0,
        "softwareVersion": "Fake Version",
        "branchId": "418",
        "services": [
            "sdwan",
            "nextgen-firewall",
            "iot-security",
            "cgnat"
        ],
        "ipAddress": "10.0.0.254",
        "startTime": "Thu Jan  1 00:00:00 1970",
        "stolenSuspected": false,
        "Hardware": {
            "name": "branch-1",
            "model": "Virtual Machine",
            "cpuCores": 0,
            "memory": "7.57GiB",
            "freeMemory": "3.81GiB",
            "diskSize": "90.34GiB",
            "freeDisk": "80.09GiB",
            "lpm": false,
            "fanless": false,
            "intelQuickAssistAcceleration": false,
            "firmwareVersion": "22.1.4",
            "manufacturer": "Microsoft Corporation",
            "serialNo": "fakeSerialNo-branch-1",
            "hardWareSerialNo": "fakeHardwareSerialNo-branch-1",
            "cpuModel": "Intel(R) Xeon(R) Platinum 8370C CPU @ 2.80GHz",
            "cpuCount": 4,
            "cpuLoad": 2,
            "interfaceCount": 1,
            "packageName": "versa-flexvnf-19700101",
            "sku": "Not Specified",
            "ssd": false
        },
        "SPack": {
            "name": "branch-1",
            "spackVersion": "418",
            "apiVersion": "11",
            "flavor": "sample",
            "releaseDate": "1970-01-01",
            "updateType": "full"
        },
        "OssPack": {
            "name": "branch-1",
            "osspackVersion": "OSSPACK Not Installed",
            "updateType": "None"
        },
        "appIdDetails": {
            "appIdInstalledEngineVersion": "3.0.0-00 ",
            "appIdInstalledBundleVersion": "1.100.0-20 "
        },
        "alarmSummary": {
            "tableId": "Alarms",
            "tableName": "Alarms",
            "monitorType": "Alarms",
            "columnNames": [
                "columnName 0"
            ],
            "rows": [
                {
                    "firstColumnValue": "critical",
                    "columnValues": [
                        2
                    ]
                },
                {
                    "firstColumnValue": "major",
                    "columnValues": [
                        2
                    ]
                },
                {
                    "firstColumnValue": "minor",
                    "columnValues": [
                        0
                    ]
                },
                {
                    "firstColumnValue": "warning",
                    "columnValues": [
                        0
                    ]
                },
                {
                    "firstColumnValue": "indeterminate",
                    "columnValues": [
                        0
                    ]
                },
                {
                    "firstColumnValue": "cleared",
                    "columnValues": [
                        6
                    ]
                }
            ]
        },
        "cpeHealth": {
            "tableName": "Appliance Health",
            "monitorType": "Health",
            "columnNames": [
                "Category",
                "Up",
                "Down"
            ],
            "rows": [
                {
                    "firstColumnValue": "Physical Ports",
                    "columnValues": [
                        0,
                        0,
                        0
                    ]
                },
                {
                    "firstColumnValue": "Config Sync Status",
                    "columnValues": [
                        0,
                        1,
                        0
                    ]
                },
                {
                    "firstColumnValue": "Reachability Status",
                    "columnValues": [
                        0,
                        1,
                        0
                    ]
                },
                {
                    "firstColumnValue": "Service Status",
                    "columnValues": [
                        0,
                        1,
                        0
                    ]
                },
                {
                    "firstColumnValue": "Interfaces",
                    "columnValues": [
                        1,
                        0,
                        0
                    ]
                },
                {
                    "firstColumnValue": "BGP Adjacencies",
                    "columnValues": [
                        2,
                        0,
                        0
                    ]
                },
                {
                    "firstColumnValue": "IKE Status",
                    "columnValues": [
                        2,
                        0,
                        0
                    ]
                },
                {
                    "firstColumnValue": "Paths",
                    "columnValues": [
                        2,
                        0,
                        0
                    ]
                }
            ]
        },
        "applicationStats": {
            "tableId": "App Activity",
            "tableName": "App Activity",
            "monitorType": "AppActivity",
            "columnNames": [
                "App Name",
                "Sessions",
                "Transactions",
                "Total BytesForward",
                "TotalBytes Reverse"
            ],
            "rows": [
                {
                    "firstColumnValue": "BITTORRENT",
                    "columnValues": [
                        1,
                        1,
                        0,
                        0
                    ]
                },
                {
                    "firstColumnValue": "ICMP",
                    "columnValues": [
                        1,
                        1,
                        0,
                        0
                    ]
                }
            ]
        },
        "policyViolation": {
            "tableId": "Policy Violation",
            "tableName": "Policy Violation",
            "monitorType": "PolicyViolation",
            "columnNames": [
                "Hit Count",
                "Packet drop no valid available link",
                "Packet drop attributed to SLA action",
                "Packet Forward attributed to SLA action"
            ],
            "rows": [
                {
                    "firstColumnValue": "datadog",
                    "columnValues": [
                        0,
                        0,
                        0,
                        0
                    ]
                }
            ]
        },
        "refreshCycleCount": 46232,
        "subType": "None",
        "branch-maintenance-mode": false,
        "applianceTags": [
            "test"
        ],
        "applianceCapabilities": {
            "capabilities": [
                "path-state-monitor",
                "bw-in-interface-state",
                "config-encryption:v4",
                "route-filter-feature",
                "internet-speed-test:v1.2"
            ]
        },
        "unreachable": true,
        "branchInMaintenanceMode": false,
        "nodes": {
            "nodeStatusList": {
                "vm-name": "NOT-APPLICABLE",
                "vm-status": "NOT-APPLICABLE",
                "node-type": "VCSN",
                "host-ip": "NOT-APPLICABLE",
                "cpu-load": 0,
                "memory-load": 0,
                "load-factor": 0,
                "slot-id": 0
            }
        },
        "ucpe-nodes": {
            "ucpeNodeStatusList": []
        }
    }
]`

// GetDirectorStatus retrieves the director status
const GetDirectorStatus = `
{
  "haConfig": {
    "clusterid": "clusterId",
    "failoverTimeout": 100,
    "slaveStartTimeout": 300,
    "autoSwitchOverTimeout": 180,
    "autoSwitchOverEnabled": false,
    "designatedMaster": true,
    "startupMode": "STANDALONE",
    "myVnfManagementIps": [
      "10.0.200.100"
    ],
    "vdsbinterfaces": [
      "10.0.201.100"
    ],
    "startupModeHA": false,
    "myNcsHaSetAsMaster": true,
    "pingViaAnyDeviceSuccessful": false,
    "peerReachableViaNcsPortAndDevices": true,
    "haEnabledOnBothNodes": false
  },
  "haDetails": {
    "enabled": false,
    "designatedMaster": true,
    "peerVnmsHaDetails": [],
    "enableHaInProgress": false
  },
  "vdSBInterfaces": [
    "10.0.201.100"
  ],
  "systemDetails": {
    "cpuCount": 32,
    "cpuLoad": "2.11",
    "memory": "64.01GB",
    "memoryFree": "20.10GB",
    "disk": "128GB",
    "diskUsage": "fakeDiskUsage"
  },
  "pkgInfo": {
    "version": "10.1",
    "packageDate": "1970101",
    "name": "versa-director-1970101-000000-vissdf0cv-10.1.0-a",
    "packageId": "vissdf0cv",
    "uiPackageId": "versa-director-1970101-000000-vissdf0cv-10.1.0-a",
    "branch": "10.1"
  },
  "systemUpTime": {
    "currentTime": "Thu Jan 01 00:00:00 UTC 1970",
    "applicationUpTime": "160 Days, 12 Hours, 56 Minutes, 35 Seconds.",
    "sysProcUptime": "230 Days, 17 Hours, 28 Minutes, 46 Seconds.",
    "sysUpTimeDetail": "20:45:35 up 230 days, 17:28,  1 users,  load average: 0.24, 0.16, 0.23"
  }
}`

// GetSLAMetrics /versa/analytics/v1.0.0/data/provider/tenants/datadog/features/SDWAN
const GetSLAMetrics = `
{
    "qTime": 1,
    "sEcho": 0,
    "iTotalDisplayRecords": 1,
    "iTotalRecords": 1,
    "aaData": [
        [
            "test-branch-2B,Controller-2,INET-1,INET-1,fc_nc",
            "test-branch-2B",
            "Controller-2",
            "INET-1",
            "INET-1",
            "fc_nc",
            101.0,
            0.0,
            0.0,
            0.0,
            0.0,
            0.0
        ]
    ]
}`

// GetLinkUsageMetrics /versa/analytics/v1.0.0/data/provider/tenants/datadog/features/SDWAN
const GetLinkUsageMetrics = `
{
    "qTime": 2,
    "sEcho": 0,
    "iTotalDisplayRecords": 1,
    "iTotalRecords": 1,
    "aaData": [
        [
            "test-branch-2B,INET-1",
            "test-branch-2B",
            "INET-1",
            "10000000000",
            "10000000000",
            "Unknown",
            "Unknown",
            "10.20.20.7",
            "",
            757144.0,
            457032.0,
            6730.168888888889,
            4062.5066666666667
        ]
    ]
}`

// GetLinkStatusMetrics /versa/analytics/v1.0.0/data/provider/tenants/datadog/features/SDWAN
const GetLinkStatusMetrics = `
{
    "qTime": 1,
    "sEcho": 0,
    "iTotalDisplayRecords": 1,
    "iTotalRecords": 1,
    "aaData": [
        [
            "test-branch-2B,INET-1",
            "test-branch-2B",
            "INET-1",
            98.5
        ]
    ]
}`

// GetSLAMetricsPage1 - First page of SLA metrics for pagination testing
const GetSLAMetricsPage1 = `
{
    "qTime": 1,
    "sEcho": 0,
    "iTotalDisplayRecords": 4,
    "iTotalRecords": 4,
    "aaData": [
        [
            "test-branch-1,test-branch-2,INET,INET,best-effort",
            "test-branch-1",
            "test-branch-2",
            "INET",
            "INET",
            "best-effort",
            120.5,
            1.2,
            1.1,
            0.001,
            0.002,
            0.0015
        ],
        [
            "test-branch-1,test-branch-3,MPLS,MPLS,real-time",
            "test-branch-1",
            "test-branch-3",
            "MPLS",
            "MPLS",
            "real-time",
            95.3,
            0.8,
            0.9,
            0.0005,
            0.0008,
            0.00065
        ]
    ]
}`

// GetSLAMetricsPage2 - Second page of SLA metrics for pagination testing
const GetSLAMetricsPage2 = `
{
    "qTime": 1,
    "sEcho": 0,
    "iTotalDisplayRecords": 4,
    "iTotalRecords": 4,
    "aaData": [
        [
            "test-branch-2,test-branch-4,INET,MPLS,best-effort",
            "test-branch-2",
            "test-branch-4",
            "INET",
            "MPLS",
            "best-effort",
            110.7,
            1.5,
            1.3,
            0.002,
            0.003,
            0.0025
        ]
    ]
}`

// GetApplicationsByApplianceMetrics /versa/analytics/v1.0.0/data/provider/tenants/datadog/features/SDWAN
const GetApplicationsByApplianceMetrics = `
{
    "qTime": 1,
    "sEcho": 0,
    "iTotalDisplayRecords": 1,
    "iTotalRecords": 1,
    "aaData": [
        [
            "test-branch-2B,HTTP",
            "test-branch-2B",
            "HTTP",
            50.0,
            1024000.0,
            512000.0,
            8192.0,
            4096.0,
            12288.0
        ]
    ]
}`

// GetTopUsers /versa/analytics/v1.0.0/data/provider/tenants/datadog/features/SDWAN
const GetTopUsers = `
{
    "qTime": 1,
    "sEcho": 0,
    "iTotalDisplayRecords": 1,
    "iTotalRecords": 1,
    "aaData": [
        [
            "test-branch-2B,testUser",
            "test-branch-2B",
            "testUser",
            50.0,
            2024000.0,
            412000.0,
            7192.0,
            2096.0,
            22288.0
        ]
    ]
}`

// GetTunnelMetrics /versa/analytics/v1.0.0/data/provider/tenants/datadog/features/SYSTEM
const GetTunnelMetrics = `
{
    "qTime": 1,
    "sEcho": 0,
    "iTotalDisplayRecords": 1,
    "iTotalRecords": 1,
    "aaData": [
        [
			"test-branch-2B,10.1.1.1",
            "test-branch-2B",
            "10.1.1.1",
            "10.2.2.2",
            "vpn-profile-1",
            67890.0,
            12345.0
        ]
    ]
}
`

// GetPathQoSMetrics /versa/analytics/v1.0.0/data/provider/tenants/datadog/features/SDWAN
const GetPathQoSMetrics = `
{
    "qTime": 1,
    "sEcho": 0,
    "iTotalDisplayRecords": 1,
    "iTotalRecords": 1,
    "aaData": [
        [
            "test-branch-2B,test-branch-2C",
            "test-branch-2B",
			"test-branch-2C",
            1000.0,
            50.0,
            2000.0,
            25.0,
            1500.0,
            75.0,
            500.0,
            10.0,
            8000000.0,
            16000000.0,
            12000000.0,
            4000000.0,
            5000.0,
            160.0,
            3.2,
            40000000.0
        ]
    ]
}
`

// GetDIAMetrics /versa/analytics/v1.0.0/data/provider/tenants/datadog/features/SDWAN
const GetDIAMetrics = `
{
    "qTime": 1,
    "sEcho": 0,
    "iTotalDisplayRecords": 1,
    "iTotalRecords": 1,
    "aaData": [
        [
            "test-branch-2B,DIA-1,192.168.1.1",
            "test-branch-2B",
            "DIA-1",
            "192.168.1.1",
            15000.0,
            12000.0,
            150000.0,
            120000.0
        ]
    ]
}
`

// GetSiteMetrics /versa/analytics/v1.0.0/data/provider/tenants/datadog/features/SDWAN
const GetSiteMetrics = `
{
    "qTime": 1,
    "sEcho": 0,
    "iTotalDisplayRecords": 1,
    "iTotalRecords": 1,
    "aaData": [
        [
            "test-branch-2B",
            "123 Main St, Anytown, USA",
            "40.7128",
            "-74.0060",
            "GPS",
            15000.0,
            12000.0,
            150000.0,
            120000.0,
            99.5
        ]
    ]
}
`
