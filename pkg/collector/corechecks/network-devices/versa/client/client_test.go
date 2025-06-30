// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package client

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/versa/client/fixtures"
	"github.com/stretchr/testify/require"
)

// TODO: add test for pagination if not moved to common functiton
func TestGetOrganizations(t *testing.T) {
	expectedOrgs := []Organization{
		{
			UUID:                    "fakeUUID",
			Name:                    "datadog",
			ParentOrg:               "fakeParentOrg",
			Connectors:              []string{"datadog-test", "datadog-other-test"},
			Plan:                    "Default-All-Services-Plan",
			GlobalOrgID:             "418", // Hyper Text Coffee Pot Control Protocol
			Description:             "DataDog Unit Test Fixture",
			SharedControlPlane:      true,
			BlockInterRegionRouting: true,
			CpeDeploymentType:       "SDWAN",
			AuthType:                "unitTest",
			ProviderOrg:             false,
			Depth:                   10,
			PushCaConfig:            false,
		},
		{
			UUID:                    "fakeUUID2",
			Name:                    "datadog2",
			ParentOrg:               "fakeParentOrg2",
			Connectors:              []string{"datadog-test", "datadog-other-test"},
			Plan:                    "Default-All-Services-Plan",
			GlobalOrgID:             "418", // Hyper Text Coffee Pot Control Protocol
			Description:             "DataDog Unit Test Fixture 2",
			SharedControlPlane:      false,
			BlockInterRegionRouting: false,
			CpeDeploymentType:       "SDWAN",
			AuthType:                "unitTest 2",
			ProviderOrg:             true,
			Depth:                   10,
			PushCaConfig:            true,
		},
	}

	server := SetupMockAPIServer()
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	actualOrgs, err := client.GetOrganizations()
	require.NoError(t, err)

	// Check contents
	require.Equal(t, 2, len(actualOrgs))
	require.Equal(t, expectedOrgs, actualOrgs)
}

func TestGetChildAppliancesDetail(t *testing.T) {
	expectedAppliances := []Appliance{
		{
			Name: "branch-1",
			UUID: "fakeUUID-branch-1",
			ApplianceLocation: ApplianceLocation{
				ApplianceName: "branch-1",
				ApplianceUUID: "fakeUUID-branch-1",
				LocationID:    "USA",
				Latitude:      "0.00",
				Longitude:     "0.00",
				Type:          "branch",
			},
			LastUpdatedTime:         "2025-04-24 20:26:11.0",
			PingStatus:              "UNREACHABLE",
			SyncStatus:              "UNKNOWN",
			YangCompatibilityStatus: "Unavailable",
			ServicesStatus:          "UNKNOWN",
			OverallStatus:           "NOT-APPLICABLE",
			PathStatus:              "Unavailable",
			IntraChassisHAStatus:    HAStatus{HAConfigured: false},
			InterChassisHAStatus:    HAStatus{HAConfigured: false},
			TemplateStatus:          "IN_SYNC",
			OwnerOrgUUID:            "another-fakeUUID-branch-1",
			OwnerOrg:                "datadog",
			Type:                    "branch",
			SngCount:                0,
			SoftwareVersion:         "Fake Version",
			BranchID:                "418",
			Services:                []string{"sdwan", "nextgen-firewall", "iot-security", "cgnat"},
			IPAddress:               "10.0.0.254",
			StartTime:               "Thu Jan  1 00:00:00 1970",
			StolenSuspected:         false,
			Hardware: Hardware{
				Name:                         "branch-1",
				Model:                        "Virtual Machine",
				CPUCores:                     0,
				Memory:                       "7.57GiB",
				FreeMemory:                   "3.81GiB",
				DiskSize:                     "90.34GiB",
				FreeDisk:                     "80.09GiB",
				LPM:                          false,
				Fanless:                      false,
				IntelQuickAssistAcceleration: false,
				FirmwareVersion:              "22.1.4",
				Manufacturer:                 "Microsoft Corporation",
				SerialNo:                     "fakeSerialNo-branch-1",
				HardWareSerialNo:             "fakeHardwareSerialNo-branch-1",
				CPUModel:                     "Intel(R) Xeon(R) Platinum 8370C CPU @ 2.80GHz",
				CPUCount:                     4,
				CPULoad:                      2,
				InterfaceCount:               1,
				PackageName:                  "versa-flexvnf-19700101",
				SKU:                          "Not Specified",
				SSD:                          false,
			},
			SPack: SPack{
				Name:         "branch-1",
				SPackVersion: "418",
				APIVersion:   "11",
				Flavor:       "sample",
				ReleaseDate:  "1970-01-01",
				UpdateType:   "full",
			},
			OssPack: OssPack{
				Name:           "branch-1",
				OssPackVersion: "OSSPACK Not Installed",
				UpdateType:     "None",
			},
			AppIDDetails: AppIDDetails{
				AppIDInstalledEngineVersion: "3.0.0-00 ",
				AppIDInstalledBundleVersion: "1.100.0-20 ",
			},
			RefreshCycleCount:       46232,
			SubType:                 "None",
			BranchMaintenanceMode:   false,
			ApplianceTags:           []string{"test"},
			ApplianceCapabilities:   CapabilitiesWrapper{Capabilities: []string{"path-state-monitor", "bw-in-interface-state", "config-encryption:v4", "route-filter-feature", "internet-speed-test:v1.2"}},
			Unreachable:             true,
			BranchInMaintenanceMode: false,
			Nodes: Nodes{
				NodeStatusList: NodeStatus{
					VMName:     "NOT-APPLICABLE",
					VMStatus:   "NOT-APPLICABLE",
					NodeType:   "VCSN",
					HostIP:     "NOT-APPLICABLE",
					CPULoad:    0,
					MemoryLoad: 0,
					LoadFactor: 0,
					SlotID:     0,
				},
			},
			UcpeNodes: UcpeNodes{UcpeNodeStatusList: []interface{}{}},
			AlarmSummary: Table{
				TableID:     "Alarms",
				TableName:   "Alarms",
				MonitorType: "Alarms",
				ColumnNames: []string{
					"columnName 0",
				},
				Rows: []TableRow{
					{
						FirstColumnValue: "critical",
						ColumnValues:     []interface{}{float64(2)},
					},
					{
						FirstColumnValue: "major",
						ColumnValues:     []interface{}{float64(2)},
					},
					{
						FirstColumnValue: "minor",
						ColumnValues:     []interface{}{float64(0)},
					},
					{
						FirstColumnValue: "warning",
						ColumnValues:     []interface{}{float64(0)},
					},
					{
						FirstColumnValue: "indeterminate",
						ColumnValues:     []interface{}{float64(0)},
					},
					{
						FirstColumnValue: "cleared",
						ColumnValues:     []interface{}{float64(6)},
					},
				},
			},
			CPEHealth: Table{
				TableName:   "Appliance Health",
				MonitorType: "Health",
				ColumnNames: []string{
					"Category",
					"Up",
					"Down",
				},
				Rows: []TableRow{
					{
						FirstColumnValue: "Physical Ports",
						ColumnValues:     []interface{}{float64(0), float64(0), float64(0)},
					},
					{
						FirstColumnValue: "Config Sync Status",
						ColumnValues:     []interface{}{float64(0), float64(1), float64(0)},
					},
					{
						FirstColumnValue: "Reachability Status",
						ColumnValues:     []interface{}{float64(0), float64(1), float64(0)},
					},
					{
						FirstColumnValue: "Service Status",
						ColumnValues:     []interface{}{float64(0), float64(1), float64(0)},
					},
					{
						FirstColumnValue: "Interfaces",
						ColumnValues:     []interface{}{float64(1), float64(0), float64(0)},
					},
					{
						FirstColumnValue: "BGP Adjacencies",
						ColumnValues:     []interface{}{float64(2), float64(0), float64(0)},
					},
					{
						FirstColumnValue: "IKE Status",
						ColumnValues:     []interface{}{float64(2), float64(0), float64(0)},
					},
					{
						FirstColumnValue: "Paths",
						ColumnValues:     []interface{}{float64(2), float64(0), float64(0)},
					},
				},
			},
			ApplicationStats: Table{
				TableID:     "App Activity",
				TableName:   "App Activity",
				MonitorType: "AppActivity",
				ColumnNames: []string{
					"App Name",
					"Sessions",
					"Transactions",
					"Total BytesForward",
					"TotalBytes Reverse",
				},
				Rows: []TableRow{
					{
						FirstColumnValue: "BITTORRENT",
						ColumnValues:     []interface{}{float64(1), float64(1), float64(0), float64(0)},
					},
					{
						FirstColumnValue: "ICMP",
						ColumnValues:     []interface{}{float64(1), float64(1), float64(0), float64(0)},
					},
				},
			},
			PolicyViolation: Table{
				TableID:     "Policy Violation",
				TableName:   "Policy Violation",
				MonitorType: "PolicyViolation",
				ColumnNames: []string{
					"Hit Count",
					"Packet drop no valid available link",
					"Packet drop attributed to SLA action",
					"Packet Forward attributed to SLA action",
				},
				Rows: []TableRow{
					{
						FirstColumnValue: "datadog",
						ColumnValues:     []interface{}{float64(0), float64(0), float64(0), float64(0)},
					},
				},
			},
		},
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		queryParams := r.URL.Query()
		if queryParams.Get("fetch") == "count" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`1`))
			return
		} else if queryParams.Get("fetch") == "all" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fixtures.GetChildAppliancesDetail))
		}
	}

	mux := setupCommonServerMux()
	mux.HandleFunc("/vnms/dashboard/childAppliancesDetail/fakeTenant", handler)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	actualAppliances, err := client.GetChildAppliancesDetail("fakeTenant")
	require.NoError(t, err)

	// Check contents
	require.Equal(t, 1, len(actualAppliances))
	require.Equal(t, expectedAppliances, actualAppliances)
}

func TestGetDirectorStatus(t *testing.T) {
	expectedDirectorStatus := &DirectorStatus{
		HAConfig: DirectorHAConfig{
			ClusterID:                      "clusterId",
			FailoverTimeout:                100,
			SlaveStartTimeout:              300,
			AutoSwitchOverTimeout:          180,
			AutoSwitchOverEnabled:          false,
			DesignatedMaster:               true,
			StartupMode:                    "STANDALONE",
			MyVnfManagementIPs:             []string{"10.0.200.100"},
			VDSBInterfaces:                 []string{"10.0.201.100"},
			StartupModeHA:                  false,
			MyNcsHaSetAsMaster:             true,
			PingViaAnyDeviceSuccessful:     false,
			PeerReachableViaNcsPortDevices: true,
			HAEnabledOnBothNodes:           false,
		},
		HADetails: DirectorHADetails{
			Enabled:            false,
			DesignatedMaster:   true,
			PeerVnmsHaDetails:  []struct{}{},
			EnableHaInProgress: false,
		},
		VDSBInterfaces: []string{"10.0.201.100"},
		SystemDetails: DirectorSystemDetails{
			CPUCount:   32,
			CPULoad:    "2.11",
			Memory:     "64.01GB",
			MemoryFree: "20.10GB",
			Disk:       "128GB",
			DiskUsage:  "fakeDiskUsage",
		},
		PkgInfo: DirectorPkgInfo{
			Version:     "10.1",
			PackageDate: "1970101",
			Name:        "versa-director-1970101-000000-vissdf0cv-10.1.0-a",
			PackageID:   "vissdf0cv",
			UIPackageID: "versa-director-1970101-000000-vissdf0cv-10.1.0-a",
			Branch:      "10.1",
		},
		SystemUpTime: DirectorSystemUpTime{
			CurrentTime:       "Thu Jan 01 00:00:00 UTC 1970",
			ApplicationUpTime: "160 Days, 12 Hours, 56 Minutes, 35 Seconds.",
			SysProcUptime:     "230 Days, 17 Hours, 28 Minutes, 46 Seconds.",
			SysUpTimeDetail:   "20:45:35 up 230 days, 17:28,  1 users,  load average: 0.24, 0.16, 0.23",
		},
	}

	server := SetupMockAPIServer()
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	actualDirectorStatus, err := client.GetDirectorStatus()
	require.NoError(t, err)

	// Check contents
	require.Equal(t, expectedDirectorStatus, actualDirectorStatus)
}

func TestGetSLAMetrics(t *testing.T) {
	expectedSLAMetrics := []SLAMetrics{
		{
			DrillKey:            "test-branch-2B,Controller-2,INET-1,INET-1,fc_nc",
			LocalSite:           "test-branch-2B",
			RemoteSite:          "Controller-2",
			LocalAccessCircuit:  "INET-1",
			RemoteAccessCircuit: "INET-1",
			ForwardingClass:     "fc_nc",
			Delay:               101.0,
			FwdDelayVar:         0.0,
			RevDelayVar:         0.0,
			FwdLossRatio:        0.0,
			RevLossRatio:        0.0,
			PDULossRatio:        0.0,
		},
	}
	server := SetupMockAPIServer()
	defer server.Close()

	client, err := testClient(server)
	// TODO: remove this override when single auth
	// method is being used
	client.directorEndpoint = server.URL
	require.NoError(t, err)

	slaMetrics, err := client.GetSLAMetrics()
	require.NoError(t, err)

	require.Equal(t, len(slaMetrics), 1)
	require.Equal(t, expectedSLAMetrics, slaMetrics)
}
