// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package serverstore

import (
	"encoding/json"
	"net/http"
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
	assert.Nil(suite.T(), rawPayloads[0].Request, "legacy append calls have no request metadata")

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

func (suite *StoreTestSuite) TestRequestMetadataSnapshots() {
	store := suite.StoreConstructor()
	defer store.Close()
	parserMap["testRoute"] = jsonParser
	defer delete(parserMap, "testRoute")
	data := []byte(`{"key":"value"}`)
	request := api.RequestMetadata{
		Method: "POST", Target: "/testRoute?probe=one", Host: "intake.example", Route: "testRoute",
		Protocol: "HTTP/1.1", ContentLength: -1, TransferEncoding: []string{"chunked"},
		Headers: http.Header{"X-Values": {"one", "two"}}, Trailers: http.Header{"X-End": {"done"}},
	}
	expected := request.Clone()
	collected := time.Unix(1700000000, 123).UTC()
	require.NoError(suite.T(), store.AppendPayload("testRoute", "key", data, "json", "application/json", collected, request))
	data[0] = 'X'
	request.Headers["X-Values"][0] = "changed"
	request.Trailers.Set("X-End", "changed")
	request.TransferEncoding[0] = "changed"

	raw := store.GetRawPayloads("testRoute")
	require.Len(suite.T(), raw, 1)
	assert.Equal(suite.T(), []byte(`{"key":"value"}`), raw[0].Data)
	assert.Equal(suite.T(), collected, raw[0].Timestamp)
	assert.Equal(suite.T(), expected, raw[0].Request)
	raw[0].Data[0] = 'X'
	raw[0].Request.Headers["X-Values"][0] = "changed"
	raw[0].Request.Trailers.Set("X-End", "changed")
	raw[0].Request.TransferEncoding[0] = "changed"

	parsed, err := GetJSONPayloads(store, "testRoute")
	require.NoError(suite.T(), err)
	require.Len(suite.T(), parsed, 1)
	assert.Equal(suite.T(), map[string]interface{}{"key": "value"}, parsed[0].Data)
	assert.Equal(suite.T(), expected, parsed[0].Request)
	parsed[0].Request.Headers.Set("X-Values", "changed")
	assert.Equal(suite.T(), expected, store.GetRawPayloads("testRoute")[0].Request)
	require.Error(suite.T(), store.AppendPayload("testRoute", "key", nil, "", "", collected, request, request))
	assert.Len(suite.T(), store.GetRawPayloads("testRoute"), 1)
}
