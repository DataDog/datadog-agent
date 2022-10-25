// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build !windows
// +build !windows

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
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func clearMetadata() {
	inventoryMutex.Lock()
	defer inventoryMutex.Unlock()

	checkMetadata = make(map[string]*checkMetadataCacheEntry)
	agentMetadata = make(AgentMetadata)

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

func TestGetPayloadForExpvar(t *testing.T) {
	ctx := context.Background()
	defer func() { clearMetadata() }()

	startNow := time.Now()
	timeNow = func() time.Time { return startNow } // time of the first run
	defer func() { timeNow = time.Now }()

	coll := mockCollector{[]check.Info{
		check.MockInfo{
			Name:         "check1",
			CheckID:      check.ID("check1_instance1"),
			Source:       "provider1",
			InitConf:     "",
			InstanceConf: "{\"test\":21}",
		},
	}}

	p := GetPayload(ctx, "testHostname", coll, false)

	assert.Equal(t, startNow.UnixNano(), p.Timestamp)

	agentMetadata := *p.AgentMetadata
	assert.NotContains(t, agentMetadata, "full_configuration")
	assert.NotContains(t, agentMetadata, "provided_configuration")

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
			CheckID:      check.ID("check1_instance1"),
			Source:       "provider1",
			InitConf:     "",
			InstanceConf: "{}",
		},
		check.MockInfo{
			Name:         "check1",
			CheckID:      check.ID("check1_instance2"),
			Source:       "provider1",
			InitConf:     "",
			InstanceConf: "{\"test\":21}",
		},
		check.MockInfo{
			Name:         "check2",
			CheckID:      check.ID("check2_instance1"),
			Source:       "provider2",
			InitConf:     "",
			InstanceConf: "{}",
		},
	}}

	SetAgentMetadata("test", true)

	SetCheckMetadata("check1_instance1", "check_provided_key1", 123)
	SetCheckMetadata("check1_instance1", "check_provided_key2", "Hi")
	SetCheckMetadata("non_running_checkid", "check_provided_key1", "this_should_not_be_kept")

	p := GetPayload(ctx, "testHostname", coll, true)

	assert.Equal(t, startNow.UnixNano(), p.Timestamp)

	agentMetadata := *p.AgentMetadata
	assert.Len(t, agentMetadata, 3) // keys are: "test", "full_configuration", "provided_configuration"
	assert.Equal(t, true, agentMetadata["test"])

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
	assert.Equal(t, "{\"test\":21}", check1Instance2["instance_config"])

	assert.Len(t, checkMeta["check2"], 1) // check2 has one instance
	check2Instance1 := *checkMeta["check2"][0]
	assert.Equal(t, "check2_instance1", check2Instance1["config.hash"])
	assert.Equal(t, "provider2", check2Instance1["config.provider"])
	assert.Equal(t, "", check2Instance1["init_config"])
	assert.Equal(t, "{}", check2Instance1["instance_config"])

	SetCheckMetadata("check2_instance1", "check_provided_key1", "hi")
	startNow = startNow.Add(1000 * time.Second)
	SetCheckMetadata("check1_instance1", "check_provided_key1", 456)

	resetFunc := setupHostMetadataMock()
	defer resetFunc()

	p = GetPayload(ctx, "testHostname", coll, true)

	assert.Equal(t, startNow.UnixNano(), p.Timestamp) //updated startNow is returned

	agentMetadata = *p.AgentMetadata
	assert.Len(t, agentMetadata, 4) // keys are: "test", "cloud_provider", "full_configuration" and "provided_configuration"
	assert.Equal(t, true, agentMetadata["test"])

	// no point in asserting every field from the agent configuration. We just check they are present and then set
	// then delete them to assert the rest.
	assert.NotNil(t, agentMetadata["provided_configuration"])
	assert.NotNil(t, agentMetadata["full_configuration"])
	delete(agentMetadata, "provided_configuration")
	delete(agentMetadata, "full_configuration")

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
					"instance_config": "{\"test\":21}"
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
		"agent_metadata":
		{
			"cloud_provider": "some_cloud_provider",
			"test": true
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
			"cloud_identifiers": {"cloud-test":"some_identifier"},
			"os_version": "testOS"
		}
	}`
	jsonString = fmt.Sprintf(jsonString, startNow.UnixNano(), version.AgentVersion)
	jsonString = strings.Join(strings.Fields(jsonString), "") // Removes whitespaces and new lines
	assert.Equal(t, jsonString, string(marshaled))

}

func TestSetup(t *testing.T) {
	defer func() { clearMetadata() }()

	startNow := time.Now()
	timeNow = func() time.Time { return startNow }
	defer func() { timeNow = time.Now }()

	timeSince = func(t time.Time) time.Duration { return 24 * time.Hour }
	defer func() { timeSince = time.Since }()

	ms := mockScheduler{
		sendNowCalled:    make(chan interface{}, 5),
		lastSendNowDelay: -1,
	}

	err := StartMetadataUpdatedGoroutine(&ms, 10*time.Minute)
	assert.Nil(t, err)

	// Collector should be added but not called after setup
	assert.False(t, waitForCalledSignal(ms.sendNowCalled))

	// New metadata should trigger the collector
	SetAgentMetadata("key", "value")
	assert.True(t, waitForCalledSignal(ms.sendNowCalled))
	assert.Equal(t, time.Duration(0), ms.lastSendNowDelay)

	// The same metadata shouldn't
	SetAgentMetadata("key", "value")
	assert.False(t, waitForCalledSignal(ms.sendNowCalled))

	// Different metadata for the same key should
	SetAgentMetadata("key", "new_value")
	assert.True(t, waitForCalledSignal(ms.sendNowCalled))
	assert.Equal(t, time.Duration(0), ms.lastSendNowDelay)

	// Simulate next call happens too quickly after the previous one
	timeSince = func(t time.Time) time.Duration { return 0 * time.Second }

	// Different metadata after a short time should trigger the collector but with a delay
	SetAgentMetadata("key", "yet_another_value")
	assert.True(t, waitForCalledSignal(ms.sendNowCalled))
	assert.True(t, ms.lastSendNowDelay > time.Duration(0))
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

// Test the `initializeConfig` function and especially its scrubbing of secret values.
func TestInitializeConfig(t *testing.T) {

	testString := func(cfgName, invName, input, output string) func(*testing.T) {
		cfg := config.NewConfig("test", "DD", strings.NewReplacer(".", "_"))
		return func(t *testing.T) {
			cfg.Set(cfgName, input)
			initializeConfig(cfg)
			require.Equal(t, output, agentMetadata[invName].(string))
		}
	}

	testStringSlice := func(cfgName, invName string, input, output []string) func(*testing.T) {
		cfg := config.NewConfig("test", "DD", strings.NewReplacer(".", "_"))
		return func(t *testing.T) {
			if input != nil {
				cfg.Set(cfgName, input)
			}
			initializeConfig(cfg)
			require.Equal(t, output, agentMetadata[invName].([]string))
		}
	}

	t.Run("config_apm_dd_url", testString(
		"apm_config.apm_dd_url",
		"config_apm_dd_url",
		"http://name:sekrit@someintake.example.com/",
		"http://name:********@someintake.example.com/",
	))

	t.Run("config_dd_url", testString(
		"dd_url",
		"config_dd_url",
		"http://name:sekrit@someintake.example.com/",
		"http://name:********@someintake.example.com/",
	))

	t.Run("config_logs_dd_url", testString(
		"logs_config.logs_dd_url",
		"config_logs_dd_url",
		"http://name:sekrit@someintake.example.com/",
		"http://name:********@someintake.example.com/",
	))

	t.Run("config_logs_socks5_proxy_address", testString(
		"logs_config.socks5_proxy_address",
		"config_logs_socks5_proxy_address",
		"http://name:sekrit@proxy.example.com/",
		"http://name:********@proxy.example.com/",
	))

	t.Run("config_no_proxy", testStringSlice(
		"proxy.no_proxy",
		"config_no_proxy",
		[]string{"http://noprox.example.com", "http://name:sekrit@proxy.example.com/"},
		[]string{"http://noprox.example.com", "http://name:********@proxy.example.com/"},
	))

	t.Run("config_no_proxy-nil", testStringSlice(
		"proxy.no_proxy",
		"config_no_proxy",
		nil,
		[]string{},
	))

	t.Run("config_process_dd_url", testString(
		"process_config.process_dd_url",
		"config_process_dd_url",
		"http://name:sekrit@someintake.example.com/",
		"http://name:********@someintake.example.com/",
	))

	t.Run("config_proxy_http", testString(
		"proxy.http",
		"config_proxy_http",
		"http://name:sekrit@proxy.example.com/",
		"http://name:********@proxy.example.com/",
	))

	t.Run("config_proxy_https", testString(
		"proxy.https",
		"config_proxy_https",
		"https://name:sekrit@proxy.example.com/",
		"https://name:********@proxy.example.com/",
	))
}
