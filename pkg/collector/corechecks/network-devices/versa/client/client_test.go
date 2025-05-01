// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

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
		},
		{
			Name: "Controller-1",
			UUID: "fakeUUID-Controller-1",
			ApplianceLocation: ApplianceLocation{
				ApplianceName: "Controller-1",
				ApplianceUUID: "fakeUUID-Controller-1",
				LocationID:    "USA",
				Latitude:      "0.00",
				Longitude:     "0.00",
				Type:          "controller",
			},
			LastUpdatedTime:         "2025-04-24 20:33:02.0",
			PingStatus:              "REACHABLE",
			SyncStatus:              "IN_SYNC",
			YangCompatibilityStatus: "Unavailable",
			ServicesStatus:          "GOOD",
			OverallStatus:           "NOT-APPLICABLE",
			PathStatus:              "Unavailable",
			IntraChassisHAStatus:    HAStatus{HAConfigured: false}, // not provided, default value
			InterChassisHAStatus:    HAStatus{HAConfigured: false},
			TemplateStatus:          "", // not provided
			OwnerOrgUUID:            "anotherFakeUUID-Controller-1",
			Type:                    "controller",
			SngCount:                0,
			SoftwareVersion:         "Fake Version",
			BranchID:                "1",
			Services:                []string{"sdwan"},
			IPAddress:               "10.0.200.100",
			StartTime:               "Thu May  2 11:32:04 2024",
			StolenSuspected:         false,
			Hardware: Hardware{
				Name:                         "Controller-1",
				Model:                        "datadog.8xlarge",
				CPUCores:                     0,
				Memory:                       "14.87GiB",
				FreeMemory:                   "4.37GiB",
				DiskSize:                     "77.30GiB",
				FreeDisk:                     "53.09GiB",
				LPM:                          false,
				Fanless:                      false,
				IntelQuickAssistAcceleration: false,
				FirmwareVersion:              "fakeFirmwareVersion",
				Manufacturer:                 "Amazon EC2",
				SerialNo:                     "fakeSerialNo",
				HardWareSerialNo:             "fakeHardwareSerialNo",
				CPUModel:                     "Intel(R) Xeon(R) Platinum 8124M CPU @ 3.00GHz",
				CPUCount:                     8,
				CPULoad:                      2,
				InterfaceCount:               3,
				PackageName:                  "fakePackageName",
				SKU:                          "Not Specified",
				SSD:                          false,
			},
			SPack: SPack{
				Name:         "Controller-1",
				SPackVersion: "418",
				APIVersion:   "161",
				Flavor:       "sample",
				ReleaseDate:  "1970-01-01",
				UpdateType:   "full",
			},
			OssPack: OssPack{
				Name:           "Controller-1",
				OssPackVersion: "19700101",
				UpdateType:     "full",
			},
			AppIDDetails: AppIDDetails{
				AppIDInstalledEngineVersion: "fakeVersion",
				AppIDInstalledBundleVersion: "fakeVersion",
			},
			RefreshCycleCount:       46233,
			SubType:                 "", // not provided
			BranchMaintenanceMode:   false,
			ApplianceTags:           []string{"Controller-1"},
			ApplianceCapabilities:   CapabilitiesWrapper{Capabilities: []string{"path-state-monitor", "bw-in-interface-state", "config-encryption:v4", "route-filter-feature", "internet-speed-test:v1.2"}},
			Unreachable:             false,
			BranchInMaintenanceMode: false,
			Nodes: Nodes{
				NodeStatusList: NodeStatus{
					VMName:     "NOT-APPLICABLE",
					VMStatus:   "NOT-APPLICABLE",
					NodeType:   "VCSN",
					HostIP:     "NOT-APPLICABLE",
					CPULoad:    2,
					MemoryLoad: 5,
					LoadFactor: 2,
					SlotID:     0,
				},
			},
			UcpeNodes: UcpeNodes{UcpeNodeStatusList: []interface{}{}},
		},
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		queryParams := r.URL.Query()
		if queryParams.Get("fetch") == "count" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`2`))
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
	require.Equal(t, 2, len(actualAppliances))
	require.Equal(t, expectedAppliances, actualAppliances)
}
