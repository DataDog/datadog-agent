// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package serverstore

import (
	"encoding/json"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

type StoreTestSuite struct {
	suite.Suite
	StoreConstructor func() Store
}

func jsonParser(p api.Payload) (interface{}, error) {
	var result map[string]interface{}
	err := json.Unmarshal(p.Data, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (suite *StoreTestSuite) TestAppendPayload() {
	store := suite.StoreConstructor()
	defer store.Close()

	data := []byte(`{"key":"value"}`)
	parserMap["testRoute"] = jsonParser
	err := store.AppendPayload("testRoute", data, "json", time.Now())
	assert.NoError(suite.T(), err)

	rawPayloads := store.GetRawPayloads("testRoute")
	assert.Len(suite.T(), rawPayloads, 1)
	assert.Equal(suite.T(), data, rawPayloads[0].Data)

	jsonPayloads, err := GetJSONPayloads(store, "testRoute")
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), jsonPayloads, 1)
	assert.Equal(suite.T(), map[string]interface{}{"key": "value"}, jsonPayloads[0].Data)
}

func (suite *StoreTestSuite) TestCleanUpPayloadsOlderThan() {
	store := suite.StoreConstructor()
	defer store.Close()

	now := time.Now()

	// Add an old payload expected to be cleaned up first
	err := store.AppendPayload("testRoute", []byte("{}"), "json", now.Add(-48*time.Hour))
	require.NoError(suite.T(), err)

	err = store.AppendPayload("testRoute", []byte("{}"), "json", now)
	require.NoError(suite.T(), err)

	rawPayloads := store.GetRawPayloads("testRoute")
	assert.Len(suite.T(), rawPayloads, 2)

	store.CleanUpPayloadsOlderThan(now.Add(-24 * time.Hour))

	rawPayloads = store.GetRawPayloads("testRoute")
	assert.Len(suite.T(), rawPayloads, 1)

	jsonPayloads, err := GetJSONPayloads(store, "testRoute")
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), jsonPayloads, 1)
}

func (suite *StoreTestSuite) TestGetRouteStats() {
	store := suite.StoreConstructor()
	defer store.Close()

	err := store.AppendPayload("routeA", []byte("{}"), "json", time.Now())
	require.NoError(suite.T(), err)

	err = store.AppendPayload("routeB", []byte("{}"), "json", time.Now())
	require.NoError(suite.T(), err)

	stats := store.GetRouteStats()

	assert.Equal(suite.T(), 1, stats["routeA"])
	assert.Equal(suite.T(), 1, stats["routeB"])
}

func (suite *StoreTestSuite) TestFlush() {
	store := suite.StoreConstructor()
	defer store.Close()

	err := store.AppendPayload("testRoute", []byte("{}"), "json", time.Now())
	require.NoError(suite.T(), err)

	store.Flush()

	rawPayloads := store.GetRawPayloads("testRoute")
	assert.Len(suite.T(), rawPayloads, 0)

	jsonPayloads, err := GetJSONPayloads(store, "testRoute")
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), jsonPayloads, 0)
}
