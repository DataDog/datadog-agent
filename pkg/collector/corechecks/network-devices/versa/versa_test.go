// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package versa

import (
	"fmt"
	"testing"

	yaml "go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/versa/client"
)

func TestFilterOrganizations(t *testing.T) {
	tts := []struct {
		description  string
		inputOrgs    []client.Organization
		includedOrgs []string
		excludedOrgs []string
		expectedOrgs map[string]struct{}
	}{
		{
			description: "Nil included and excluded organizations",
			inputOrgs: []client.Organization{
				{Name: "org1"},
				{Name: "org2"},
				{Name: "org3"},
			},
			includedOrgs: nil,
			excludedOrgs: nil,
			expectedOrgs: map[string]struct{}{
				"org1": {},
				"org2": {},
				"org3": {},
			},
		},
		{
			description: "Empty included and excluded organizations",
			inputOrgs: []client.Organization{
				{Name: "org1"},
				{Name: "org2"},
				{Name: "org3"},
			},
			includedOrgs: []string{},
			excludedOrgs: []string{},
			expectedOrgs: map[string]struct{}{
				"org1": {},
				"org2": {},
				"org3": {},
			},
		},
		{
			description: "Input and included organizations do not intersect",
			inputOrgs: []client.Organization{
				{Name: "org1"},
				{Name: "org2"},
				{Name: "org3"},
			},
			includedOrgs: []string{"org4", "org5"},
			excludedOrgs: []string{},
			expectedOrgs: map[string]struct{}{},
		},
		{
			description: "Included organizations include some of the input",
			inputOrgs: []client.Organization{
				{Name: "org1"},
				{Name: "org2"},
				{Name: "org3"},
			},
			includedOrgs: []string{"org4", "org2", "org1"},
			excludedOrgs: []string{},
			expectedOrgs: map[string]struct{}{
				"org1": {},
				"org2": {},
			},
		},
		{
			description: "Included organizations and input match",
			inputOrgs: []client.Organization{
				{Name: "org1"},
				{Name: "org2"},
				{Name: "org3"},
			},
			includedOrgs: []string{"org1", "org2", "org3"},
			excludedOrgs: []string{},
			expectedOrgs: map[string]struct{}{
				"org1": {},
				"org2": {},
				"org3": {},
			},
		},
		{
			description: "Test Case Insensitive Matching",
			inputOrgs: []client.Organization{
				{Name: "ORG1"},
				{Name: "Org2"},
				{Name: "orG3"},
			},
			includedOrgs: []string{"Org1", "ORG2", "oRG3"},
			excludedOrgs: []string{"OrG3"},
			expectedOrgs: map[string]struct{}{
				"ORG1": {},
				"Org2": {},
				// "orG3": {}, // This should be excluded because it's on the included and excluded list
			},
		},
		{
			description: "Excluded organizations match input test case insensitivity",
			inputOrgs: []client.Organization{
				{Name: "ORG1"},
				{Name: "Org2"},
				{Name: "orG3"},
			},
			includedOrgs: []string{},
			excludedOrgs: []string{"ORG2", "Org1"},
			expectedOrgs: map[string]struct{}{
				"orG3": {},
			},
		},
		{
			description: "Both included and excluded organizations match input",
			inputOrgs: []client.Organization{
				{Name: "org1"},
				{Name: "org2"},
				{Name: "org3"},
				{Name: "org4"},
			},
			includedOrgs: []string{"org1", "org2", "org3", "org4"},
			excludedOrgs: []string{"org2", "org3"},
			expectedOrgs: map[string]struct{}{
				"org1": {},
				"org4": {},
			},
		},
	}

	for _, test := range tts {
		t.Run(test.description, func(t *testing.T) {
			actualOrgs := filterOrganizations(test.inputOrgs, test.includedOrgs, test.excludedOrgs)
			if len(actualOrgs) != len(test.expectedOrgs) {
				t.Errorf("Unexpected number of organizations: expected %d: %v, got %d: %v", len(test.expectedOrgs), test.expectedOrgs, len(actualOrgs), actualOrgs)
			}
			actualOrgsSet := make(map[string]struct{})
			for _, org := range actualOrgs {
				actualOrgsSet[org.Name] = struct{}{}
			}
			for expectedOrg := range test.expectedOrgs {
				if _, ok := actualOrgsSet[expectedOrg]; !ok {
					t.Errorf("Expected organization %s not found in actual organizations", expectedOrg)
				}
			}
		})
	}
}

func TestGenerateDeviceNameToIDMap(t *testing.T) {
	tts := []struct {
		description string
		devices     []metadata.DeviceMetadata
		expectedMap map[string]string
	}{
		{
			description: "Empty devices list",
			devices:     []metadata.DeviceMetadata{},
			expectedMap: map[string]string{},
		},
		{
			description: "Multiple devices with names",
			devices: []metadata.DeviceMetadata{
				{
					IPAddress: "1",
					Name:      "branch1",
				},
				{
					IPAddress: "2",
					Name:      "branch2",
				},
			},
			expectedMap: map[string]string{
				"branch1": "1",
				"branch2": "2",
			},
		},
	}
	for _, test := range tts {
		t.Run(test.description, func(t *testing.T) {
			actualMap := generateDeviceNameToIPMap(test.devices)
			if fmt.Sprint(actualMap) != fmt.Sprint(test.expectedMap) {
				t.Errorf("Expected map %v, got %v", test.expectedMap, actualMap)
			}
		})
	}
}

func TestFilterAppliances(t *testing.T) {
	tts := []struct {
		description        string
		inputAppliances    []client.Appliance
		includedDevices    []string
		excludedDevices    []string
		expectedAppliances map[string]struct{}
	}{
		{
			description: "Nil included and excluded devices monitors everything",
			inputAppliances: []client.Appliance{
				{Name: "branch1"},
				{Name: "branch2"},
			},
			includedDevices: nil,
			excludedDevices: nil,
			expectedAppliances: map[string]struct{}{
				"branch1": {},
				"branch2": {},
			},
		},
		{
			description: "Empty included and excluded devices monitors everything",
			inputAppliances: []client.Appliance{
				{Name: "branch1"},
				{Name: "branch2"},
			},
			includedDevices: []string{},
			excludedDevices: []string{},
			expectedAppliances: map[string]struct{}{
				"branch1": {},
				"branch2": {},
			},
		},
		{
			description: "Single device opt-in selects only that appliance",
			inputAppliances: []client.Appliance{
				{Name: "branch1"},
				{Name: "branch2"},
				{Name: "branch3"},
			},
			includedDevices: []string{"branch2"},
			excludedDevices: nil,
			expectedAppliances: map[string]struct{}{
				"branch2": {},
			},
		},
		{
			description: "Included devices that do not intersect the input yield nothing",
			inputAppliances: []client.Appliance{
				{Name: "branch1"},
				{Name: "branch2"},
			},
			includedDevices:    []string{"branch9"},
			excludedDevices:    nil,
			expectedAppliances: map[string]struct{}{},
		},
		{
			description: "Included devices match case insensitively",
			inputAppliances: []client.Appliance{
				{Name: "NDM-Controller3"},
				{Name: "branch2"},
			},
			includedDevices: []string{"ndm-controller3"},
			excludedDevices: nil,
			expectedAppliances: map[string]struct{}{
				"NDM-Controller3": {},
			},
		},
		{
			description: "Excluded devices drop matching appliances",
			inputAppliances: []client.Appliance{
				{Name: "branch1"},
				{Name: "BRANCH2"},
				{Name: "branch3"},
			},
			includedDevices: nil,
			excludedDevices: []string{"branch2"},
			expectedAppliances: map[string]struct{}{
				"branch1": {},
				"branch3": {},
			},
		},
		{
			description: "Exclusion wins when a device is in both lists",
			inputAppliances: []client.Appliance{
				{Name: "branch1"},
				{Name: "branch2"},
			},
			includedDevices: []string{"branch1", "branch2"},
			excludedDevices: []string{"branch2"},
			expectedAppliances: map[string]struct{}{
				"branch1": {},
			},
		},
	}

	for _, test := range tts {
		t.Run(test.description, func(t *testing.T) {
			actualAppliances := filterAppliances(test.inputAppliances, test.includedDevices, test.excludedDevices)
			if len(actualAppliances) != len(test.expectedAppliances) {
				t.Errorf("Unexpected number of appliances: expected %d: %v, got %d: %v", len(test.expectedAppliances), test.expectedAppliances, len(actualAppliances), actualAppliances)
			}
			for _, appliance := range actualAppliances {
				if _, ok := test.expectedAppliances[appliance.Name]; !ok {
					t.Errorf("Unexpected appliance %q in filtered results", appliance.Name)
				}
			}
		})
	}
}

func TestFilterInterfacesByDevice(t *testing.T) {
	interfaces := []client.Interface{
		{DeviceName: "branch1", Name: "vni-0/0"},
		{DeviceName: "branch2", Name: "vni-0/1"},
		{DeviceName: "voae-cluster", Name: "eth0"},
	}

	tts := []struct {
		description        string
		monitoredDevices   map[string]struct{}
		directorName       string
		expectedInterfaces map[string]struct{}
	}{
		{
			description:      "A nil monitored device set disables filtering",
			monitoredDevices: nil,
			directorName:     "voae-cluster",
			expectedInterfaces: map[string]struct{}{
				"vni-0/0": {},
				"vni-0/1": {},
				"eth0":    {},
			},
		},
		{
			description:      "Interfaces of filtered out appliances are dropped",
			monitoredDevices: map[string]struct{}{"branch1": {}},
			directorName:     "voae-cluster",
			expectedInterfaces: map[string]struct{}{
				"vni-0/0": {},
				"eth0":    {},
			},
		},
		{
			description:      "Director interfaces survive even when no appliance is monitored",
			monitoredDevices: map[string]struct{}{},
			directorName:     "voae-cluster",
			expectedInterfaces: map[string]struct{}{
				"eth0": {},
			},
		},
		{
			description:      "Director name is matched case insensitively",
			monitoredDevices: map[string]struct{}{},
			directorName:     "VOAE-Cluster",
			expectedInterfaces: map[string]struct{}{
				"eth0": {},
			},
		},
		{
			description:        "An empty director name drops director interfaces",
			monitoredDevices:   map[string]struct{}{},
			directorName:       "",
			expectedInterfaces: map[string]struct{}{},
		},
	}

	for _, test := range tts {
		t.Run(test.description, func(t *testing.T) {
			actualInterfaces := filterInterfacesByDevice(interfaces, test.monitoredDevices, test.directorName)
			if len(actualInterfaces) != len(test.expectedInterfaces) {
				t.Errorf("Unexpected number of interfaces: expected %d: %v, got %d: %v", len(test.expectedInterfaces), test.expectedInterfaces, len(actualInterfaces), actualInterfaces)
			}
			for _, iface := range actualInterfaces {
				if _, ok := test.expectedInterfaces[iface.Name]; !ok {
					t.Errorf("Unexpected interface %q in filtered results", iface.Name)
				}
			}
		})
	}
}

func TestMonitoredDeviceNames(t *testing.T) {
	tts := []struct {
		description     string
		appliances      []client.Appliance
		includedDevices []string
		excludedDevices []string
		expectedNames   map[string]struct{}
	}{
		{
			description:     "No device filtering configured returns nil to disable filtering",
			appliances:      []client.Appliance{{Name: "Branch1"}, {Name: "branch2"}},
			includedDevices: nil,
			excludedDevices: nil,
			expectedNames:   nil,
		},
		{
			description:     "Configured filtering lowercases the surviving appliance names",
			appliances:      []client.Appliance{{Name: "Branch1"}, {Name: "branch2"}},
			includedDevices: []string{"branch1", "branch2"},
			excludedDevices: nil,
			expectedNames:   map[string]struct{}{"branch1": {}, "branch2": {}},
		},
		{
			description:     "Configured filtering with no surviving appliances returns an empty, non-nil set",
			appliances:      []client.Appliance{},
			includedDevices: []string{"branch1"},
			excludedDevices: nil,
			expectedNames:   map[string]struct{}{},
		},
		{
			description:     "Only excluded_devices set still enables filtering",
			appliances:      []client.Appliance{{Name: "branch1"}},
			includedDevices: nil,
			excludedDevices: []string{"branch2"},
			expectedNames:   map[string]struct{}{"branch1": {}},
		},
	}

	for _, test := range tts {
		t.Run(test.description, func(t *testing.T) {
			check := &VersaCheck{config: checkCfg{
				IncludedDevices: deviceEntries(test.includedDevices...),
				ExcludedDevices: test.excludedDevices,
			}}

			actualNames := check.monitoredDeviceNames(test.appliances)
			if test.expectedNames == nil {
				if actualNames != nil {
					t.Errorf("Expected nil monitored devices to disable filtering, got %v", actualNames)
				}
				return
			}
			if actualNames == nil {
				t.Fatalf("Expected %v, got nil", test.expectedNames)
			}
			if len(actualNames) != len(test.expectedNames) {
				t.Errorf("Expected %v, got %v", test.expectedNames, actualNames)
			}
			for name := range test.expectedNames {
				if _, ok := actualNames[name]; !ok {
					t.Errorf("Expected monitored device %q to be present in %v", name, actualNames)
				}
			}
		})
	}
}

func TestFilterAnalyticsMetricsByDevice(t *testing.T) {
	slaMetrics := []client.SLAMetrics{
		{LocalSite: "branch1"},
		{LocalSite: "BRANCH2"},
		{LocalSite: "branch3"},
	}

	tts := []struct {
		description      string
		monitoredDevices map[string]struct{}
		expectedSites    map[string]struct{}
	}{
		{
			description:      "A nil monitored device set disables filtering",
			monitoredDevices: nil,
			expectedSites: map[string]struct{}{
				"branch1": {},
				"BRANCH2": {},
				"branch3": {},
			},
		},
		{
			description:      "Results for unmonitored devices are dropped",
			monitoredDevices: map[string]struct{}{"branch1": {}},
			expectedSites: map[string]struct{}{
				"branch1": {},
			},
		},
		{
			description:      "Result device names are matched case insensitively",
			monitoredDevices: map[string]struct{}{"branch2": {}},
			expectedSites: map[string]struct{}{
				"BRANCH2": {},
			},
		},
		{
			description:      "An empty monitored device set drops every result",
			monitoredDevices: map[string]struct{}{},
			expectedSites:    map[string]struct{}{},
		},
	}

	for _, test := range tts {
		t.Run(test.description, func(t *testing.T) {
			actualMetrics := filterAnalyticsMetricsByDevice(slaMetrics, test.monitoredDevices, func(m client.SLAMetrics) string { return m.LocalSite })
			if len(actualMetrics) != len(test.expectedSites) {
				t.Errorf("Unexpected number of results: expected %d: %v, got %d: %v", len(test.expectedSites), test.expectedSites, len(actualMetrics), actualMetrics)
			}
			for _, metric := range actualMetrics {
				if _, ok := test.expectedSites[metric.LocalSite]; !ok {
					t.Errorf("Unexpected result for site %q in filtered results", metric.LocalSite)
				}
			}
		})
	}
}

// deviceEntries builds included_devices entries from bare names, for tests that do not
// care about per-device tags.
func deviceEntries(names ...string) []deviceEntry {
	if names == nil {
		return nil
	}
	entries := make([]deviceEntry, 0, len(names))
	for _, name := range names {
		entries = append(entries, deviceEntry{Name: name})
	}
	return entries
}

func TestDeviceEntryUnmarshalYAML(t *testing.T) {
	tts := []struct {
		description     string
		rawConfig       string
		expectedEntries []deviceEntry
		expectError     bool
	}{
		{
			description:     "A plain string list still parses",
			rawConfig:       "included_devices:\n  - branch1\n  - branch2\n",
			expectedEntries: []deviceEntry{{Name: "branch1"}, {Name: "branch2"}},
		},
		{
			description: "A mapping carries name and tags",
			rawConfig:   "included_devices:\n  - name: branch1\n    tags:\n      env: prod\n      site_type: retail\n",
			expectedEntries: []deviceEntry{{
				Name: "branch1",
				Tags: map[string]string{"env": "prod", "site_type": "retail"},
			}},
		},
		{
			description: "Both forms can be mixed in one list",
			rawConfig:   "included_devices:\n  - branch1\n  - name: branch2\n    tags:\n      env: staging\n",
			expectedEntries: []deviceEntry{
				{Name: "branch1"},
				{Name: "branch2", Tags: map[string]string{"env": "staging"}},
			},
		},
		{
			description:     "A mapping without tags is accepted",
			rawConfig:       "included_devices:\n  - name: branch1\n",
			expectedEntries: []deviceEntry{{Name: "branch1"}},
		},
		{
			description: "A mapping without a name is rejected",
			rawConfig:   "included_devices:\n  - tags:\n      env: prod\n",
			expectError: true,
		},
	}

	for _, test := range tts {
		t.Run(test.description, func(t *testing.T) {
			var config checkCfg
			err := yaml.Unmarshal([]byte(test.rawConfig), &config)
			if test.expectError {
				if err == nil {
					t.Fatalf("Expected an error, got config %+v", config.IncludedDevices)
				}
				return
			}
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if fmt.Sprint(config.IncludedDevices) != fmt.Sprint(test.expectedEntries) {
				t.Errorf("Expected %+v, got %+v", test.expectedEntries, config.IncludedDevices)
			}
		})
	}
}

func TestConfiguredDeviceTags(t *testing.T) {
	tts := []struct {
		description     string
		includedDevices []deviceEntry
		expectedTags    map[string][]string
	}{
		{
			description:     "No configured tags yields an empty map",
			includedDevices: []deviceEntry{{Name: "branch1"}},
			expectedTags:    map[string][]string{},
		},
		{
			description: "Tags are rendered as sorted key:value strings keyed by lowercased name",
			includedDevices: []deviceEntry{{
				Name: "Branch1",
				Tags: map[string]string{"site_type": "retail", "env": "prod"},
			}},
			expectedTags: map[string][]string{
				"branch1": {"env:prod", "site_type:retail"},
			},
		},
		{
			description: "Devices without tags are omitted while tagged ones are kept",
			includedDevices: []deviceEntry{
				{Name: "branch1"},
				{Name: "branch2", Tags: map[string]string{"env": "staging"}},
			},
			expectedTags: map[string][]string{
				"branch2": {"env:staging"},
			},
		},
		{
			description: "An empty tag key is dropped",
			includedDevices: []deviceEntry{{
				Name: "branch1",
				Tags: map[string]string{"": "prod", "env": "staging"},
			}},
			expectedTags: map[string][]string{
				"branch1": {"env:staging"},
			},
		},
		{
			description: "An empty tag value is preserved",
			includedDevices: []deviceEntry{{
				Name: "branch1",
				Tags: map[string]string{"env": ""},
			}},
			expectedTags: map[string][]string{
				"branch1": {"env:"},
			},
		},
	}

	for _, test := range tts {
		t.Run(test.description, func(t *testing.T) {
			check := &VersaCheck{config: checkCfg{IncludedDevices: test.includedDevices}}
			actualTags := check.configuredDeviceTags()
			if fmt.Sprint(actualTags) != fmt.Sprint(test.expectedTags) {
				t.Errorf("Expected %v, got %v", test.expectedTags, actualTags)
			}
		})
	}
}

func TestApplyConfiguredDeviceTags(t *testing.T) {
	tts := []struct {
		description    string
		deviceMetadata []metadata.DeviceMetadata
		configuredTags map[string][]string
		expectedTags   map[string][]string
	}{
		{
			description: "Configured tags are appended to the matching device only",
			deviceMetadata: []metadata.DeviceMetadata{
				{Name: "branch1", Tags: []string{"device_vendor:versa"}},
				{Name: "branch2", Tags: []string{"device_vendor:versa"}},
			},
			configuredTags: map[string][]string{"branch1": {"env:prod"}},
			expectedTags: map[string][]string{
				"branch1": {"device_vendor:versa", "env:prod"},
				"branch2": {"device_vendor:versa"},
			},
		},
		{
			description: "Device names are matched case insensitively",
			deviceMetadata: []metadata.DeviceMetadata{
				{Name: "Branch1", Tags: []string{"device_vendor:versa"}},
			},
			configuredTags: map[string][]string{"branch1": {"env:prod"}},
			expectedTags: map[string][]string{
				"Branch1": {"device_vendor:versa", "env:prod"},
			},
		},
		{
			description: "Tags configured for an absent device are ignored",
			deviceMetadata: []metadata.DeviceMetadata{
				{Name: "branch1", Tags: []string{"device_vendor:versa"}},
			},
			configuredTags: map[string][]string{"branch9": {"env:prod"}},
			expectedTags: map[string][]string{
				"branch1": {"device_vendor:versa"},
			},
		},
		{
			description: "No configured tags leaves metadata untouched",
			deviceMetadata: []metadata.DeviceMetadata{
				{Name: "branch1", Tags: []string{"device_vendor:versa"}},
			},
			configuredTags: nil,
			expectedTags: map[string][]string{
				"branch1": {"device_vendor:versa"},
			},
		},
	}

	for _, test := range tts {
		t.Run(test.description, func(t *testing.T) {
			applyConfiguredDeviceTags(test.deviceMetadata, test.configuredTags)
			for _, device := range test.deviceMetadata {
				expected, ok := test.expectedTags[device.Name]
				if !ok {
					t.Fatalf("Unexpected device %q in results", device.Name)
				}
				if fmt.Sprint(device.Tags) != fmt.Sprint(expected) {
					t.Errorf("Device %q: expected tags %v, got %v", device.Name, expected, device.Tags)
				}
			}
		})
	}
}
