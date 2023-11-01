// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build !windows

package inventories

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func clearMetadata() {
	inventoryMutex.Lock()
	defer inventoryMutex.Unlock()

	checkMetadata = make(map[string]*checkMetadataCacheEntry)

	// purge metadataUpdatedC
L:
	for {
		select {
		case <-metadataUpdatedC:
		default: // To make sure this call is not blocking
			break L
		}
	}
}

type mockScheduler struct {
	sendNowCalled    chan interface{}
	lastSendNowDelay time.Duration
}

func (m *mockScheduler) AddCollector(name string, interval time.Duration) error {
	return nil
}

func (m *mockScheduler) TriggerAndResetCollectorTimer(name string, delay time.Duration) {
	m.lastSendNowDelay = delay
	m.sendNowCalled <- nil
}

func waitForCalledSignal(calledSignal chan interface{}) bool {
	select {
	case <-calledSignal:
		return true
	case <-time.After(1 * time.Second):
		return false
	}
}

func TestRemoveCheckMetadata(t *testing.T) {
	defer func() { clearMetadata() }()

	SetCheckMetadata("check1", "check_provided_key1", 123)
	SetCheckMetadata("check2", "check_provided_key1", 123)

	RemoveCheckMetadata("check1")

	assert.Len(t, checkMetadata, 1)
	assert.Contains(t, checkMetadata, "check2")
}

type mockCollector struct {
	Checks []check.Info
}

func (m mockCollector) MapOverChecks(fn func([]check.Info)) {
	fn(m.Checks)
}

func (m mockCollector) GetChecks() []check.Check {
	return nil
}

func TestGetPayloadForExpvar(t *testing.T) {
	ctx := context.Background()
	defer func() { clearMetadata() }()

	startNow := time.Now()
	timeNow = func() time.Time { return startNow } // time of the first run
	defer func() { timeNow = time.Now }()

	coll := mockCollector{[]check.Info{
		check.MockInfo{
			Name:         "check1",
			CheckID:      checkid.ID("check1_instance1"),
			Source:       "provider1",
			InitConf:     "",
			InstanceConf: "{\"test\":21}",
		},
	}}

	p := GetPayload(ctx, "testHostname", coll, false)

	assert.Equal(t, startNow.UnixNano(), p.Timestamp)

	checkMeta := *p.CheckMetadata
	require.Len(t, checkMeta, 1)
	require.Len(t, checkMeta["check1"], 1)

	check1Instance1 := *checkMeta["check1"][0]
	assert.Equal(t, "check1_instance1", check1Instance1["config.hash"])
	assert.Equal(t, "provider1", check1Instance1["config.provider"])
	assert.NotContains(t, check1Instance1, "init_config")
	assert.NotContains(t, check1Instance1, "instance_config")
}

func TestGetPayload(t *testing.T) {
	ctx := context.Background()
	defer func() { clearMetadata() }()

	cfg := config.Mock(t)
	cfg.Set("inventories_configuration_enabled", true)
	cfg.Set("inventories_checks_configuration_enabled", true)

	startNow := time.Now()
	timeNow = func() time.Time { return startNow } // time of the first run
	defer func() { timeNow = time.Now }()

	coll := mockCollector{[]check.Info{
		check.MockInfo{
			Name:         "check1",
			CheckID:      checkid.ID("check1_instance1"),
			Source:       "provider1",
			InitConf:     "",
			InstanceConf: "{}",
		},
		check.MockInfo{
			Name:         "check1",
			CheckID:      checkid.ID("check1_instance2"),
			Source:       "provider1",
			InitConf:     "",
			InstanceConf: "{\"test\":21}",
		},
		check.MockInfo{
			Name:         "check2",
			CheckID:      checkid.ID("check2_instance1"),
			Source:       "provider2",
			InitConf:     "",
			InstanceConf: "{}",
		},
	}}

	SetCheckMetadata("check1_instance1", "check_provided_key1", 123)
	SetCheckMetadata("check1_instance1", "check_provided_key2", "Hi")
	SetCheckMetadata("non_running_checkid", "check_provided_key1", "this_should_not_be_kept")

	p := GetPayload(ctx, "testHostname", coll, true)

	assert.Equal(t, startNow.UnixNano(), p.Timestamp)

	checkMeta := *p.CheckMetadata
	assert.Len(t, checkMeta, 2)           // 'non_running_checkid' should have been cleaned
	assert.Len(t, checkMeta["check1"], 2) // check1 has two instances

	check1Instance1 := *checkMeta["check1"][0]
	assert.Equal(t, "check1_instance1", check1Instance1["config.hash"])
	assert.Equal(t, "provider1", check1Instance1["config.provider"])
	assert.Equal(t, 123, check1Instance1["check_provided_key1"])
	assert.Equal(t, "Hi", check1Instance1["check_provided_key2"])
	assert.Equal(t, "", check1Instance1["init_config"])
	assert.Equal(t, "{}", check1Instance1["instance_config"])

	check1Instance2 := *checkMeta["check1"][1]
	assert.Equal(t, "check1_instance2", check1Instance2["config.hash"])
	assert.Equal(t, "provider1", check1Instance2["config.provider"])
	assert.Equal(t, "", check1Instance2["init_config"])
	assert.Equal(t, "test: 21", check1Instance2["instance_config"])

	assert.Len(t, checkMeta["check2"], 1) // check2 has one instance
	check2Instance1 := *checkMeta["check2"][0]
	assert.Equal(t, "check2_instance1", check2Instance1["config.hash"])
	assert.Equal(t, "provider2", check2Instance1["config.provider"])
	assert.Equal(t, "", check2Instance1["init_config"])
	assert.Equal(t, "{}", check2Instance1["instance_config"])

	SetCheckMetadata("check2_instance1", "check_provided_key1", "hi")
	startNow = startNow.Add(1000 * time.Second)
	SetCheckMetadata("check1_instance1", "check_provided_key1", 456)

	setupHostMetadataMock(t)

	p = GetPayload(ctx, "testHostname", coll, true)

	assert.Equal(t, startNow.UnixNano(), p.Timestamp) //updated startNow is returned

	checkMeta = *p.CheckMetadata
	assert.Len(t, checkMeta, 2)
	check1Instance1 = *checkMeta["check1"][0]
	assert.Equal(t, "check1_instance1", check1Instance1["config.hash"])
	assert.Equal(t, "provider1", check1Instance1["config.provider"])
	assert.Equal(t, 456, check1Instance1["check_provided_key1"]) //Key has been updated
	assert.Equal(t, "Hi", check1Instance1["check_provided_key2"])
	check1Instance2 = *checkMeta["check1"][1]
	assert.Equal(t, "check1_instance2", check1Instance2["config.hash"])
	assert.Equal(t, "provider1", check1Instance2["config.provider"])
	check2Instance1 = *checkMeta["check2"][0]
	assert.Equal(t, "check2_instance1", check2Instance1["config.hash"])
	assert.Equal(t, "provider2", check2Instance1["config.provider"])
	assert.Equal(t, "hi", check2Instance1["check_provided_key1"]) // New key added

	marshaled, err := p.MarshalJSON()
	assert.Nil(t, err)
	jsonString := `
	{
		"hostname": "testHostname",
		"timestamp": %v,
		"check_metadata":
		{
			"check1":
			[
				{
					"check_provided_key1": 456,
					"check_provided_key2": "Hi",
					"config.hash": "check1_instance1",
					"config.provider": "provider1",
					"init_config": "",
					"instance_config": "{}"
				},
				{
					"config.hash": "check1_instance2",
					"config.provider": "provider1",
					"init_config": "",
					"instance_config": "test: 21"
				}
			],
			"check2":
			[
				{
					"check_provided_key1": "hi",
					"config.hash": "check2_instance1",
					"config.provider": "provider2",
					"init_config": "",
					"instance_config": "{}"
				}
			]
		},
		"host_metadata":
		{
			"cpu_cores": 6,
			"cpu_logical_processors": 6,
			"cpu_vendor": "GenuineIntel",
			"cpu_model": "Intel_i7-8750H",
			"cpu_model_id": "158",
			"cpu_family": "6",
			"cpu_stepping": "10",
			"cpu_frequency": 2208.006,
			"cpu_cache_size": 9437184,
			"kernel_name": "Linux",
			"kernel_release": "5.17.0-1-amd64",
			"kernel_version": "Debian_5.17.3-1",
			"os": "GNU/Linux",
			"cpu_architecture": "unknown",
			"memory_total_kb": 1205632,
			"memory_swap_total_kb": 1205632,
			"ip_address": "192.168.24.138",
			"ipv6_address": "fe80::20c:29ff:feb6:d232",
			"mac_address": "00:0c:29:b6:d2:32",
			"agent_version": "%v",
			"cloud_provider": "some_cloud_provider",
			"cloud_provider_source": "",
			"cloud_provider_account_id": "",
			"cloud_provider_host_id": "",
			"os_version": "testOS",
			"hypervisor_guest_uuid": "hypervisorUUID",
			"dmi_product_uuid": "dmiUUID",
			"dmi_board_asset_tag": "boardTag",
			"dmi_board_vendor": "boardVendor"
		}
	}`
	jsonString = fmt.Sprintf(jsonString, startNow.UnixNano(), version.AgentVersion)
	// jsonString above is structure for easy editing, we have to convert if to a compact JSON
	jsonString = strings.Replace(jsonString, "\t", "", -1)      // Removes tabs
	jsonString = strings.Replace(jsonString, "\n", "", -1)      // Removes line breaks
	jsonString = strings.Replace(jsonString, "\": ", "\":", -1) // Remove space between keys and values
	assert.Equal(t, jsonString, string(marshaled))

}

func TestCreateCheckInstanceMetadataReturnsNewMetadata(t *testing.T) {
	defer clearMetadata()

	const (
		checkID        = "a-check-id"
		configProvider = "a-config-provider"
		metadataKey    = "a-metadata-key"
	)

	checkMetadata[checkID] = &checkMetadataCacheEntry{
		CheckInstanceMetadata: CheckInstanceMetadata{
			metadataKey: "a-metadata-value",
		},
	}

	md := createCheckInstanceMetadata(checkID, configProvider, "", "", false)
	(*md)[metadataKey] = "a-different-metadata-value"

	assert.NotEqual(t, checkMetadata[checkID].CheckInstanceMetadata[metadataKey], (*md)[metadataKey])
}
