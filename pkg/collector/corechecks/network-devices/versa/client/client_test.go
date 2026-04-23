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

func TestGetTopology(t *testing.T) {
	expectedTopology := []Neighbor{
		{
			
		},
	}

	server := SetupMockAPIServer()
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	actualTopology, err := client.GetTopology("test-branch-2B")
	require.NoError(t, err)

	// Check contents
	require.Equal(t, expectedTopology, actualTopology)

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

	slaMetrics, err := client.GetSLAMetrics("datadog")
	require.NoError(t, err)

	require.Equal(t, len(slaMetrics), 1)
	require.Equal(t, expectedSLAMetrics, slaMetrics)
}

func TestGetLinkUsageMetrics(t *testing.T) {
	expectedLinkUsageMetrics := []LinkUsageMetrics{
		{
			DrillKey:          "test-branch-2B,INET-1",
			Site:              "test-branch-2B",
			AccessCircuit:     "INET-1",
			UplinkBandwidth:   "10000000000",
			DownlinkBandwidth: "10000000000",
			Type:              "Unknown",
			Media:             "Unknown",
			IP:                "10.20.20.7",
			ISP:               "",
			VolumeTx:          757144.0,
			VolumeRx:          457032.0,
			BandwidthTx:       6730.168888888889,
			BandwidthRx:       4062.5066666666667,
		},
	}
	server := SetupMockAPIServer()
	defer server.Close()

	client, err := testClient(server)
	// TODO: remove this override when single auth
	// method is being used
	client.directorEndpoint = server.URL
	require.NoError(t, err)

	linkUsageMetrics, err := client.GetLinkUsageMetrics("datadog")
	require.NoError(t, err)

	require.Equal(t, len(linkUsageMetrics), 1)
	require.Equal(t, expectedLinkUsageMetrics, linkUsageMetrics)
}

func TestGetLinkStatusMetrics(t *testing.T) {
	expectedLinkStatusMetrics := []LinkStatusMetrics{
		{
			DrillKey:      "test-branch-2B,INET-1",
			Site:          "test-branch-2B",
			AccessCircuit: "INET-1",
			Availability:  98.5,
		},
	}
	server := SetupMockAPIServer()
	defer server.Close()

	client, err := testClient(server)
	// TODO: remove this override when single auth
	// method is being used
	client.directorEndpoint = server.URL
	require.NoError(t, err)

	linkStatusMetrics, err := client.GetLinkStatusMetrics("datadog")
	require.NoError(t, err)

	require.Equal(t, len(linkStatusMetrics), 1)
	require.Equal(t, expectedLinkStatusMetrics, linkStatusMetrics)
}

func TestGetQoSMetrics(t *testing.T) {
	expectedQoSMetrics := []QoSMetrics{
		{
			DrillKey:             "test-branch-2B,test-branch-2C",
			LocalSiteName:        "test-branch-2B",
			RemoteSiteName:       "test-branch-2C",
			BestEffortTx:         1000.0,
			BestEffortTxDrop:     50.0,
			ExpeditedForwardTx:   2000.0,
			ExpeditedForwardDrop: 25.0,
			AssuredForwardTx:     1500.0,
			AssuredForwardDrop:   75.0,
			NetworkControlTx:     500.0,
			NetworkControlDrop:   10.0,
			BestEffortBandwidth:  8000000.0,
			ExpeditedForwardBW:   16000000.0,
			AssuredForwardBW:     12000000.0,
			NetworkControlBW:     4000000.0,
			VolumeTx:             5000.0,
			TotalDrop:            160.0,
			PercentDrop:          3.2,
			Bandwidth:            40000000.0,
		},
	}
	server := SetupMockAPIServer()
	defer server.Close()

	client, err := testClient(server)
	// TODO: remove this override when single auth
	// method is being used
	client.directorEndpoint = server.URL
	require.NoError(t, err)

	qosMetrics, err := client.GetPathQoSMetrics("datadog")
	require.NoError(t, err)

	require.Equal(t, len(qosMetrics), 1)
	require.Equal(t, expectedQoSMetrics, qosMetrics)
}
func TestGetDIAMetrics(t *testing.T) {
	expectedDIAMetrics := []DIAMetrics{
		{
			DrillKey:      "test-branch-2B,DIA-1,192.168.1.1",
			Site:          "test-branch-2B",
			AccessCircuit: "DIA-1",
			IP:            "192.168.1.1",
			VolumeTx:      15000.0,
			VolumeRx:      12000.0,
			BandwidthTx:   150000.0,
			BandwidthRx:   120000.0,
		},
	}
	server := SetupMockAPIServer()
	defer server.Close()

	client, err := testClient(server)
	// TODO: remove this override when single auth
	// method is being used
	client.directorEndpoint = server.URL
	require.NoError(t, err)

	diaMetrics, err := client.GetDIAMetrics("datadog")
	require.NoError(t, err)

	require.Equal(t, len(diaMetrics), 1)
	require.Equal(t, expectedDIAMetrics, diaMetrics)
}

func TestGetSiteMetrics(t *testing.T) {
	expectedSiteMetrics := []SiteMetrics{
		{
			Site:           "test-branch-2B",
			Address:        "123 Main St, Anytown, USA",
			Latitude:       "40.7128",
			Longitude:      "-74.0060",
			LocationSource: "GPS",
			VolumeTx:       15000.0,
			VolumeRx:       12000.0,
			BandwidthTx:    150000.0,
			BandwidthRx:    120000.0,
			Availability:   99.5,
		},
	}
	server := SetupMockAPIServer()
	defer server.Close()

	client, err := testClient(server)
	// TODO: remove this override when single auth
	// method is being used
	client.directorEndpoint = server.URL
	require.NoError(t, err)

	siteMetrics, err := client.GetSiteMetrics("datadog")
	require.NoError(t, err)

	require.Equal(t, len(siteMetrics), 1)
	require.Equal(t, expectedSiteMetrics, siteMetrics)
}

func TestGetApplicationsByAppliance(t *testing.T) {
	expectedApplicationsByApplianceMetrics := []ApplicationsByApplianceMetrics{
		{
			DrillKey:    "test-branch-2B,HTTP",
			Site:        "test-branch-2B",
			AppID:       "HTTP",
			Sessions:    50.0,
			VolumeTx:    1024000.0,
			VolumeRx:    512000.0,
			BandwidthTx: 8192.0,
			BandwidthRx: 4096.0,
			Bandwidth:   12288.0,
		},
	}
	server := SetupMockAPIServer()
	defer server.Close()

	client, err := testClient(server)
	// TODO: remove this override when single auth
	// method is being used
	client.directorEndpoint = server.URL
	require.NoError(t, err)

	appsByApplianceMetrics, err := client.GetApplicationsByAppliance("datadog")
	require.NoError(t, err)

	require.Equal(t, len(appsByApplianceMetrics), 1)
	require.Equal(t, expectedApplicationsByApplianceMetrics, appsByApplianceMetrics)
}

func TestGetTunnelMetrics(t *testing.T) {
	expectedTunnelMetrics := []TunnelMetrics{
		{
			DrillKey:    "test-branch-2B,10.1.1.1",
			Appliance:   "test-branch-2B",
			LocalIP:     "10.1.1.1",
			RemoteIP:    "10.2.2.2",
			VpnProfName: "vpn-profile-1",
			VolumeRx:    67890.0,
			VolumeTx:    12345.0,
		},
	}
	server := SetupMockAPIServer()
	defer server.Close()

	client, err := testClient(server)
	// TODO: remove this override when single auth
	// method is being used
	client.directorEndpoint = server.URL
	require.NoError(t, err)

	tunnelMetrics, err := client.GetTunnelMetrics("datadog")
	require.NoError(t, err)

	require.Equal(t, len(tunnelMetrics), 1)
	require.Equal(t, expectedTunnelMetrics, tunnelMetrics)
}

func TestGetTunnelMetricsEmptyTenant(t *testing.T) {
	server := SetupMockAPIServer()
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	_, err = client.GetTunnelMetrics("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "tenant cannot be empty")
}

func TestGetTopUsers(t *testing.T) {
	expectedTopUsers := []TopUserMetrics{
		{
			DrillKey:    "test-branch-2B,testUser",
			Site:        "test-branch-2B",
			User:        "testUser",
			Sessions:    50.0,
			VolumeTx:    2024000.0,
			VolumeRx:    412000.0,
			BandwidthTx: 7192.0,
			BandwidthRx: 2096.0,
			Bandwidth:   22288.0,
		},
	}
	server := SetupMockAPIServer()
	defer server.Close()

	client, err := testClient(server)
	// TODO: remove this override when single auth
	// method is being used
	client.directorEndpoint = server.URL
	require.NoError(t, err)

	topUsers, err := client.GetTopUsers("datadog")
	require.NoError(t, err)

	require.Equal(t, len(topUsers), 1)
	require.Equal(t, expectedTopUsers, topUsers)
}

func TestGetSLAMetricsPagination(t *testing.T) {
	expectedSLAMetrics := []SLAMetrics{
		// First page results
		{
			DrillKey:            "test-branch-1,test-branch-2,INET,INET,best-effort",
			LocalSite:           "test-branch-1",
			RemoteSite:          "test-branch-2",
			LocalAccessCircuit:  "INET",
			RemoteAccessCircuit: "INET",
			ForwardingClass:     "best-effort",
			Delay:               120.5,
			FwdDelayVar:         1.2,
			RevDelayVar:         1.1,
			FwdLossRatio:        0.001,
			RevLossRatio:        0.002,
			PDULossRatio:        0.0015,
		},
		{
			DrillKey:            "test-branch-1,test-branch-3,MPLS,MPLS,real-time",
			LocalSite:           "test-branch-1",
			RemoteSite:          "test-branch-3",
			LocalAccessCircuit:  "MPLS",
			RemoteAccessCircuit: "MPLS",
			ForwardingClass:     "real-time",
			Delay:               95.3,
			FwdDelayVar:         0.8,
			RevDelayVar:         0.9,
			FwdLossRatio:        0.0005,
			RevLossRatio:        0.0008,
			PDULossRatio:        0.00065,
		},
		// Second page results
		{
			DrillKey:            "test-branch-2,test-branch-4,INET,MPLS,best-effort",
			LocalSite:           "test-branch-2",
			RemoteSite:          "test-branch-4",
			LocalAccessCircuit:  "INET",
			RemoteAccessCircuit: "MPLS",
			ForwardingClass:     "best-effort",
			Delay:               110.7,
			FwdDelayVar:         1.5,
			RevDelayVar:         1.3,
			FwdLossRatio:        0.002,
			RevLossRatio:        0.003,
			PDULossRatio:        0.0025,
		},
	}

	server := SetupPaginationMockAPIServer()
	defer server.Close()

	// Create client with small maxCount to force pagination
	client, err := testClient(server)
	require.NoError(t, err)

	// Override client settings to test pagination
	client.maxCount = "2" // Small page size to force pagination
	client.maxPages = 5   // Allow enough pages

	// TODO: remove this override when single auth method is being used
	client.directorEndpoint = server.URL
	require.NoError(t, err)

	slaMetrics, err := client.GetSLAMetrics("datadog")
	require.NoError(t, err)

	require.Equal(t, len(expectedSLAMetrics), len(slaMetrics))
	require.Equal(t, expectedSLAMetrics, slaMetrics)
}

func TestGetSLAMetricsPaginationWithMaxPages(t *testing.T) {
	server := SetupPaginationMockAPIServer()
	defer server.Close()

	// Create client with maxPages limit to test early termination
	client, err := testClient(server)
	require.NoError(t, err)

	// Override client settings - limit to 1 page only
	client.maxCount = "2" // Small page size
	client.maxPages = 1   // Only allow 1 page

	// TODO: remove this override when single auth method is being used
	client.directorEndpoint = server.URL
	require.NoError(t, err)

	slaMetrics, err := client.GetSLAMetrics("datadog")
	require.NoError(t, err)

	// Should only get results from first page (2 items)
	require.Equal(t, 2, len(slaMetrics))
	require.Equal(t, "test-branch-1", slaMetrics[0].LocalSite)
	require.Equal(t, "test-branch-1", slaMetrics[1].LocalSite)
}

func TestGetSLAMetricsPaginationEmptyResponse(t *testing.T) {
	server := SetupPaginationMockAPIServer()
	defer server.Close()

	// Create client that will request beyond available data
	client, err := testClient(server)
	require.NoError(t, err)

	// Override client settings to start from a high offset
	client.maxCount = "100" // Large page size to get all data in first page
	client.maxPages = 5     // Allow enough pages

	// TODO: remove this override when single auth method is being used
	client.directorEndpoint = server.URL
	require.NoError(t, err)

	slaMetrics, err := client.GetSLAMetrics("datadog")
	require.NoError(t, err)

	// Should get all available data in first page and stop
	require.Equal(t, 3, len(slaMetrics)) // Total of 3 items across all pages
}

func TestGetAnalyticsInterfaces(t *testing.T) {
	expectedAnalyticsInterfaceMetrics := []AnalyticsInterfaceMetrics{
		{
			DrillKey:    "test-branch-2B,INET-1,ge-0/0/1",
			Site:        "test-branch-2B",
			AccessCkt:   "INET-1",
			Interface:   "ge-0/0/1",
			RxUtil:      25.5,
			TxUtil:      18.3,
			VolumeRx:    1024000.0,
			VolumeTx:    768000.0,
			Volume:      1792000.0,
			BandwidthRx: 8192.0,
			BandwidthTx: 6144.0,
			Bandwidth:   14336.0,
		},
	}
	server := SetupMockAPIServer()
	defer server.Close()

	client, err := testClient(server)
	// TODO: remove this override when single auth
	// method is being used
	client.directorEndpoint = server.URL
	require.NoError(t, err)

	analyticsInterfaceMetrics, err := client.GetAnalyticsInterfaces("datadog")
	require.NoError(t, err)

	require.Equal(t, len(analyticsInterfaceMetrics), 1)
	require.Equal(t, expectedAnalyticsInterfaceMetrics, analyticsInterfaceMetrics)
}

func TestGetAnalyticsInterfacesEmptyTenant(t *testing.T) {
	server := SetupMockAPIServer()
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	_, err = client.GetAnalyticsInterfaces("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "tenant cannot be empty")
}
