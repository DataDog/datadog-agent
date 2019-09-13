// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build !windows

package inventories

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/stretchr/testify/assert"
)

func mockGetLoadedConfigs() map[string]integration.Config {
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

func mockGetAllInstanceIDs(checkName string) []check.ID {
	if checkName == "check1" {
		return []check.ID{"check1_instance1", "check1_instance2"}
	} else if checkName == "check2" {
		return []check.ID{"check2_instance1"}
	}
	return nil
}

func TestGetPayload(t *testing.T) {
	getLoadedConfigs = mockGetLoadedConfigs
	getAllInstanceIDs = mockGetAllInstanceIDs
	startNow := time.Now().UnixNano()
	nowNano = func() int64 { return startNow } // time of the first run
	defer func() {
		getLoadedConfigs = common.AC.GetLoadedConfigs
		getAllInstanceIDs = common.Coll.GetAllInstanceIDs
		nowNano = time.Now().UnixNano
	}()

	SetAgentMetadata("test", true)
	SetCheckMetadata("check1_instance1", "check_provided_key1", 123)
	SetCheckMetadata("check1_instance1", "check_provided_key2", "Hi")
	SetCheckMetadata("non_running_checkid", "check_provided_key1", "this should get deleted")

	p := GetPayload()

	assert.Equal(t, startNow, p.Timestamp)

	agentMetadata := *p.AgentMetadata
	assert.Len(t, agentMetadata, 1)
	assert.Equal(t, true, agentMetadata["test"])

	checkMetadata := *p.CheckMetadata
	assert.Len(t, checkMetadata, 2)           // non_running_checkid is not there
	assert.Len(t, checkMetadata["check1"], 2) // check1 has two instances
	check1Instance1 := *checkMetadata["check1"][0]
	assert.Len(t, check1Instance1, 5)
	assert.Equal(t, startNow, check1Instance1["last_updated"])
	assert.Equal(t, "check1_instance1", check1Instance1["config.hash"])
	assert.Equal(t, "provider1", check1Instance1["config.provider"])
	assert.Equal(t, 123, check1Instance1["check_provided_key1"])
	assert.Equal(t, "Hi", check1Instance1["check_provided_key2"])
	check1Instance2 := *checkMetadata["check1"][1]
	assert.Len(t, check1Instance2, 3)
	assert.Equal(t, agentStartupTime, check1Instance2["last_updated"])
	assert.Equal(t, "check1_instance2", check1Instance2["config.hash"])
	assert.Equal(t, "provider1", check1Instance2["config.provider"])
	assert.Len(t, checkMetadata["check2"], 1) // check2 has one instance
	check2Instance1 := *checkMetadata["check2"][0]
	assert.Len(t, check2Instance1, 3)
	assert.Equal(t, agentStartupTime, check2Instance1["last_updated"])
	assert.Equal(t, "check2_instance1", check2Instance1["config.hash"])
	assert.Equal(t, "provider2", check2Instance1["config.provider"])

	SetCheckMetadata("check2_instance1", "check_provided_key1", "hi")
	originalStartNow := startNow
	startNow += 1000
	SetCheckMetadata("check1_instance1", "check_provided_key1", 456)

	p = GetPayload()

	assert.Equal(t, startNow, p.Timestamp) //updated startNow is returned

	agentMetadata = *p.AgentMetadata
	assert.Len(t, agentMetadata, 1)
	assert.Equal(t, true, agentMetadata["test"])

	checkMetadata = *p.CheckMetadata
	assert.Len(t, checkMetadata, 2)
	check1Instance1 = *checkMetadata["check1"][0]
	assert.Len(t, check1Instance1, 5)
	assert.Equal(t, startNow, check1Instance1["last_updated"]) // last_updated has changed
	assert.Equal(t, "check1_instance1", check1Instance1["config.hash"])
	assert.Equal(t, "provider1", check1Instance1["config.provider"])
	assert.Equal(t, 456, check1Instance1["check_provided_key1"]) //Key has been updated
	assert.Equal(t, "Hi", check1Instance1["check_provided_key2"])
	check1Instance2 = *checkMetadata["check1"][1]
	assert.Len(t, check1Instance2, 3)
	assert.Equal(t, agentStartupTime, check1Instance2["last_updated"]) // last_updated still the same
	assert.Equal(t, "check1_instance2", check1Instance2["config.hash"])
	assert.Equal(t, "provider1", check1Instance2["config.provider"])
	check2Instance1 = *checkMetadata["check2"][0]
	assert.Len(t, check2Instance1, 4)
	assert.Equal(t, originalStartNow, check2Instance1["last_updated"]) // reflects when check_provided_key1 was changed
	assert.Equal(t, "check2_instance1", check2Instance1["config.hash"])
	assert.Equal(t, "provider2", check2Instance1["config.provider"])
	assert.Equal(t, "hi", check2Instance1["check_provided_key1"]) // New key added

}
