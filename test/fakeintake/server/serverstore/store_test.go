// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package serverstore

import (
	"encoding/json"
	"fmt"
	"testing"
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

	rawPayloads := store.GetRawPayloads("testRoute")
	assert.Len(suite.T(), rawPayloads, 1)
	assert.Equal(suite.T(), data, rawPayloads[0].Data)
	assert.Equal(suite.T(), "1234", rawPayloads[0].APIKey)

	jsonPayloads, err := GetJSONPayloads(store, "testRoute")
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

	rawPayloads := store.GetRawPayloads("testRoute")
	assert.Len(suite.T(), rawPayloads, 0)

	jsonPayloads, err := GetJSONPayloads(store, "testRoute")
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), jsonPayloads, 0)
}

func TestInMemoryStoreGetRawPayloadsAfter(t *testing.T) {
	store := newInMemoryStore()

	// Append 5 payloads to "api/v1/metrics"
	for i := 0; i < 5; i++ {
		err := store.AppendPayload("api/v1/metrics", "", []byte(fmt.Sprintf("metric-%d", i)), "", "application/json", time.Now())
		require.NoError(t, err)
	}

	// First fetch with cursor=0: should return all 5 payloads, cursor=5
	payloads, cursor := store.GetRawPayloadsAfter("api/v1/metrics", 0)
	assert.Equal(t, 5, cursor)
	assert.Len(t, payloads, 5)

	// Second fetch with cursor=5: should return 0 payloads (nothing new), cursor=5
	payloads, cursor = store.GetRawPayloadsAfter("api/v1/metrics", 5)
	assert.Equal(t, 5, cursor)
	assert.Empty(t, payloads)

	// Append 3 more payloads
	for i := 5; i < 8; i++ {
		err := store.AppendPayload("api/v1/metrics", "", []byte(fmt.Sprintf("metric-%d", i)), "", "application/json", time.Now())
		require.NoError(t, err)
	}

	// Third fetch with cursor=5: should return 3 new payloads, cursor=8
	payloads, cursor = store.GetRawPayloadsAfter("api/v1/metrics", 5)
	assert.Equal(t, 8, cursor)
	assert.Len(t, payloads, 3)

	// Fetch with cursor=8: nothing new
	payloads, cursor = store.GetRawPayloadsAfter("api/v1/metrics", 8)
	assert.Equal(t, 8, cursor)
	assert.Empty(t, payloads)
}

func TestInMemoryStoreGetRawPayloadsAfterWithCleanup(t *testing.T) {
	store := newInMemoryStore()

	// Append 10 payloads
	for i := 0; i < 10; i++ {
		err := store.AppendPayload("api/v1/metrics", "", []byte(fmt.Sprintf("metric-%d", i)), "", "application/json", time.Now())
		require.NoError(t, err)
	}

	// Client fetches all 10: cursor=10
	payloads, cursor := store.GetRawPayloadsAfter("api/v1/metrics", 0)
	assert.Equal(t, 10, cursor)
	assert.Len(t, payloads, 10)

	// Simulate cleanup: remove the first 4 payloads (older than retention)
	// We do this by directly manipulating the store's internal state.
	store.mutex.Lock()
	store.rawPayloads["api/v1/metrics"] = store.rawPayloads["api/v1/metrics"][4:]
	store.mutex.Unlock()
	// totalAppended is still 10, len(rawPayloads) is now 6
	// cleanedUp = 10 - 6 = 4

	// Append 2 new payloads (totalAppended becomes 12)
	for i := 10; i < 12; i++ {
		err := store.AppendPayload("api/v1/metrics", "", []byte(fmt.Sprintf("metric-%d", i)), "", "application/json", time.Now())
		require.NoError(t, err)
	}

	// Client fetches with cursor=10: should get only the 2 new payloads
	// cleanedUp = 12 - 8 = 4, start = 10 - 4 = 6, current has 8 payloads
	// payloads[6:] = 2 payloads (the 2 new ones)
	payloads, cursor = store.GetRawPayloadsAfter("api/v1/metrics", 10)
	assert.Equal(t, 12, cursor)
	assert.Len(t, payloads, 2)
}

func TestInMemoryStoreGetRawPayloadsAfterFlush(t *testing.T) {
	store := newInMemoryStore()

	for i := 0; i < 5; i++ {
		err := store.AppendPayload("api/v1/metrics", "", []byte(fmt.Sprintf("metric-%d", i)), "", "application/json", time.Now())
		require.NoError(t, err)
	}

	// Fetch to set cursor
	_, cursor := store.GetRawPayloadsAfter("api/v1/metrics", 0)
	assert.Equal(t, 5, cursor)

	// Flush should reset totalAppended
	store.Flush()

	// After flush, cursor should be 0 and all payloads returned
	payloads, cursor := store.GetRawPayloadsAfter("api/v1/metrics", 0)
	assert.Equal(t, 0, cursor)
	assert.Empty(t, payloads)

	// Append new payloads after flush
	for i := 0; i < 3; i++ {
		err := store.AppendPayload("api/v1/metrics", "", []byte(fmt.Sprintf("metric-%d", i)), "", "application/json", time.Now())
		require.NoError(t, err)
	}

	payloads, cursor = store.GetRawPayloadsAfter("api/v1/metrics", 0)
	assert.Equal(t, 3, cursor)
	assert.Len(t, payloads, 3)
}

func TestInMemoryStoreGetRawPayloadsAfterUnknownRoute(t *testing.T) {
	store := newInMemoryStore()

	// Unknown route: should return 0 payloads, cursor=0
	payloads, cursor := store.GetRawPayloadsAfter("unknown", 0)
	assert.Equal(t, 0, cursor)
	assert.Empty(t, payloads)

	// Even with a non-zero cursor on an unknown route
	payloads, cursor = store.GetRawPayloadsAfter("unknown", 100)
	assert.Equal(t, 0, cursor)
	assert.Empty(t, payloads)
}
