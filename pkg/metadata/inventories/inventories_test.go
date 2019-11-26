// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build !windows

package inventories

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/stretchr/testify/assert"
)

func clearMetadata() {
	checkCacheMutex.Lock()
	defer checkCacheMutex.Unlock()
	checkMetadataCache = make(map[string]*checkMetadataCacheEntry)
	agentCacheMutex.Lock()
	defer agentCacheMutex.Unlock()
	agentMetadataCache = make(AgentMetadata)

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

type mockAutoConfig struct{}

func (*mockAutoConfig) GetLoadedConfigs() map[string]integration.Config {
	ret := make(map[string]integration.Config)
	ret["check1_digest"] = integration.Config{
		Name:     "check1",
		Provider: "provider1",
	}
	ret["check2_digest"] = integration.Config{
		Name:     "check2",
		Provider: "provider2",
	}
	return ret
}

type mockCollector struct{}

func (*mockCollector) GetAllInstanceIDs(checkName string) []check.ID {
	if checkName == "check1" {
		return []check.ID{"check1_instance1", "check1_instance2"}
	} else if checkName == "check2" {
		return []check.ID{"check2_instance1"}
	}
	return nil
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

func TestGetPayload(t *testing.T) {
	defer func() { clearMetadata() }()

	startNow := time.Now()
	timeNow = func() time.Time { return startNow } // time of the first run
	defer func() { timeNow = time.Now }()

	SetAgentMetadata("test", true)
	SetCheckMetadata("check1_instance1", "check_provided_key1", 123)
	SetCheckMetadata("check1_instance1", "check_provided_key2", "Hi")
	SetCheckMetadata("non_running_checkid", "check_provided_key1", "this_should_be_kept")

	p := GetPayload("testHostname", &mockAutoConfig{}, &mockCollector{})

	assert.Equal(t, startNow.UnixNano(), p.Timestamp)

	agentMetadata := *p.AgentMetadata
	assert.Len(t, agentMetadata, 1)
	assert.Equal(t, true, agentMetadata["test"])

	checkMetadata := *p.CheckMetadata
	assert.Len(t, checkMetadata, 3)
	assert.Len(t, checkMetadata["check1"], 2) // check1 has two instances
	check1Instance1 := *checkMetadata["check1"][0]
	assert.Len(t, check1Instance1, 5)
	assert.Equal(t, startNow.UnixNano(), check1Instance1["last_updated"])
	assert.Equal(t, "check1_instance1", check1Instance1["config.hash"])
	assert.Equal(t, "provider1", check1Instance1["config.provider"])
	assert.Equal(t, 123, check1Instance1["check_provided_key1"])
	assert.Equal(t, "Hi", check1Instance1["check_provided_key2"])
	check1Instance2 := *checkMetadata["check1"][1]
	assert.Len(t, check1Instance2, 3)
	assert.Equal(t, agentStartupTime.UnixNano(), check1Instance2["last_updated"])
	assert.Equal(t, "check1_instance2", check1Instance2["config.hash"])
	assert.Equal(t, "provider1", check1Instance2["config.provider"])
	assert.Len(t, checkMetadata["check2"], 1) // check2 has one instance
	check2Instance1 := *checkMetadata["check2"][0]
	assert.Len(t, check2Instance1, 3)
	assert.Equal(t, agentStartupTime.UnixNano(), check2Instance1["last_updated"])
	assert.Equal(t, "check2_instance1", check2Instance1["config.hash"])
	assert.Equal(t, "provider2", check2Instance1["config.provider"])

	SetCheckMetadata("check2_instance1", "check_provided_key1", "hi")
	originalStartNow := startNow
	startNow = startNow.Add(1000 * time.Second)
	SetCheckMetadata("check1_instance1", "check_provided_key1", 456)

	p = GetPayload("testHostname", &mockAutoConfig{}, &mockCollector{})

	assert.Equal(t, startNow.UnixNano(), p.Timestamp) //updated startNow is returned

	agentMetadata = *p.AgentMetadata
	assert.Len(t, agentMetadata, 1)
	assert.Equal(t, true, agentMetadata["test"])

	checkMetadata = *p.CheckMetadata
	assert.Len(t, checkMetadata, 3)
	check1Instance1 = *checkMetadata["check1"][0]
	assert.Len(t, check1Instance1, 5)
	assert.Equal(t, startNow.UnixNano(), check1Instance1["last_updated"]) // last_updated has changed
	assert.Equal(t, "check1_instance1", check1Instance1["config.hash"])
	assert.Equal(t, "provider1", check1Instance1["config.provider"])
	assert.Equal(t, 456, check1Instance1["check_provided_key1"]) //Key has been updated
	assert.Equal(t, "Hi", check1Instance1["check_provided_key2"])
	check1Instance2 = *checkMetadata["check1"][1]
	assert.Len(t, check1Instance2, 3)
	assert.Equal(t, agentStartupTime.UnixNano(), check1Instance2["last_updated"]) // last_updated still the same
	assert.Equal(t, "check1_instance2", check1Instance2["config.hash"])
	assert.Equal(t, "provider1", check1Instance2["config.provider"])
	check2Instance1 = *checkMetadata["check2"][0]
	assert.Len(t, check2Instance1, 4)
	assert.Equal(t, originalStartNow.UnixNano(), check2Instance1["last_updated"]) // reflects when check_provided_key1 was changed
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
					"last_updated": %v
				},
				{
					"config.hash": "check1_instance2",
					"config.provider": "provider1",
					"last_updated": %v
				}
			],
			"check2":
			[
				{
					"check_provided_key1": "hi",
					"config.hash": "check2_instance1",
					"config.provider": "provider2",
					"last_updated": %v
				}
			],
			"non_running_checkid":
			[
				{
					"check_provided_key1": "this_should_be_kept",
					"config.hash": "non_running_checkid",
					"config.provider": "",
					"last_updated": %v
				}
			]
		},
		"agent_metadata":
		{
			"test": true
		}
	}`
	jsonString = fmt.Sprintf(jsonString, startNow.UnixNano(), startNow.UnixNano(), agentStartupTime.UnixNano(), originalStartNow.UnixNano(), originalStartNow.UnixNano())
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
