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
	defer delete(parserMap, "testRoute")
	err := store.AppendPayload("testRoute", "1234", data, "json", "", time.Now())
	assert.NoError(suite.T(), err)

	rawPayloads, hasMore := store.GetRawPayloads("testRoute", 0, 0)
	assert.Len(suite.T(), rawPayloads, 1)
	assert.Equal(suite.T(), data, rawPayloads[0].Data)
	assert.Equal(suite.T(), "1234", rawPayloads[0].APIKey)
	assert.False(suite.T(), hasMore)

	jsonPayloads, _, _, err := GetJSONPayloads(store, "testRoute", 0, 0)
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), jsonPayloads, 1)
	assert.Equal(suite.T(), map[string]interface{}{"key": "value"}, jsonPayloads[0].Data)
}

func (suite *StoreTestSuite) TestCleanUpPayloadsOlderThan() {
	store := suite.StoreConstructor()
	defer store.Close()

	now := time.Now()

	parserMap["testRoute"] = jsonParser
	defer delete(parserMap, "testRoute")
	// Add an old payload expected to be cleaned up first
	err := store.AppendPayload("testRoute", "1234", []byte("{}"), "json", "", now.Add(-48*time.Hour))
	require.NoError(suite.T(), err)

	err = store.AppendPayload("testRoute", "1234", []byte("{}"), "json", "", now)
	require.NoError(suite.T(), err)

	rawPayloads, _ := store.GetRawPayloads("testRoute", 0, 0)
	assert.Len(suite.T(), rawPayloads, 2)

	store.CleanUpPayloadsOlderThan(now.Add(-24 * time.Hour))

	rawPayloads, _ = store.GetRawPayloads("testRoute", 0, 0)
	assert.Len(suite.T(), rawPayloads, 1)

	jsonPayloads, _, _, err := GetJSONPayloads(store, "testRoute", 0, 0)
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), jsonPayloads, 1)
}

// TestGetRawPayloadsPagination tests GetRawPayloads' page boundary behavior.
func (suite *StoreTestSuite) TestGetRawPayloadsPagination() {
	store := suite.StoreConstructor()
	defer store.Close()

	for i := 0; i < 5; i++ {
		err := store.AppendPayload("testRoute", "1234", []byte("{}"), "json", "", time.Now())
		require.NoError(suite.T(), err)
	}

	payloads, hasMore := store.GetRawPayloads("testRoute", 0, 0)
	assert.Len(suite.T(), payloads, 5)
	assert.False(suite.T(), hasMore)

	payloads, hasMore = store.GetRawPayloads("testRoute", 0, 2)
	require.Len(suite.T(), payloads, 2)
	assert.Equal(suite.T(), uint64(1), payloads[0].Seq)
	assert.Equal(suite.T(), uint64(2), payloads[1].Seq)
	assert.True(suite.T(), hasMore)

	payloads, hasMore = store.GetRawPayloads("testRoute", 2, 3)
	require.Len(suite.T(), payloads, 3)
	assert.Equal(suite.T(), uint64(3), payloads[0].Seq)
	assert.Equal(suite.T(), uint64(5), payloads[2].Seq)
	assert.False(suite.T(), hasMore)

	payloads, hasMore = store.GetRawPayloads("testRoute", 2, 100)
	assert.Len(suite.T(), payloads, 3)
	assert.False(suite.T(), hasMore)

	payloads, hasMore = store.GetRawPayloads("testRoute", 5, 10)
	assert.Empty(suite.T(), payloads)
	assert.False(suite.T(), hasMore)
}

// TestGetRawPayloadsCursorStableAcrossCleanup tests that a cursor taken before CleanUpPayloadsOlderThan 
// trims the front of the store still returns every remaining payload with no gaps or duplicates.
func (suite *StoreTestSuite) TestGetRawPayloadsCursorStableAcrossCleanup() {
	store := suite.StoreConstructor()
	defer store.Close()

	now := time.Now()
	err := store.AppendPayload("testRoute", "1234", []byte("{}"), "json", "", now.Add(-48*time.Hour))
	require.NoError(suite.T(), err)
	err = store.AppendPayload("testRoute", "1234", []byte("{}"), "json", "", now.Add(-48*time.Hour))
	require.NoError(suite.T(), err)
	err = store.AppendPayload("testRoute", "1234", []byte("{}"), "json", "", now)
	require.NoError(suite.T(), err)

	// a client has only paged through the first (soon to be trimmed) payload
	cursor := uint64(1)

	store.CleanUpPayloadsOlderThan(now.Add(-24 * time.Hour))

	payloads, hasMore := store.GetRawPayloads("testRoute", cursor, 10)
	require.Len(suite.T(), payloads, 1)
	assert.Equal(suite.T(), uint64(3), payloads[0].Seq)
	assert.False(suite.T(), hasMore)
}

func (suite *StoreTestSuite) TestGetRouteStats() {
	store := suite.StoreConstructor()
	defer store.Close()

	err := store.AppendPayload("routeA", "1234", []byte("{}"), "json", "", time.Now())
	require.NoError(suite.T(), err)

	err = store.AppendPayload("routeB", "1234", []byte("{}"), "json", "", time.Now())
	require.NoError(suite.T(), err)

	stats := store.GetRouteStats()

	assert.Equal(suite.T(), 1, stats["routeA"])
	assert.Equal(suite.T(), 1, stats["routeB"])
}

func (suite *StoreTestSuite) TestFlush() {
	store := suite.StoreConstructor()
	defer store.Close()

	parserMap["testRoute"] = jsonParser
	defer delete(parserMap, "testRoute")
	err := store.AppendPayload("testRoute", "1234", []byte("{}"), "json", "", time.Now())
	require.NoError(suite.T(), err)

	store.Flush()

	rawPayloads, _ := store.GetRawPayloads("testRoute", 0, 0)
	assert.Len(suite.T(), rawPayloads, 0)

	jsonPayloads, _, _, err := GetJSONPayloads(store, "testRoute", 0, 0)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), jsonPayloads, 0)
}
