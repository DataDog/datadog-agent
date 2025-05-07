// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

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
