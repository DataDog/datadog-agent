// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package serverstore

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

func TestSqlStore(t *testing.T) {
	suite.Run(t, &StoreTestSuite{
		StoreConstructor: func() Store {
			return NewSQLStore()
		},
	})
}

func TestExecuteQuery(t *testing.T) {
	store := NewSQLStore()
	defer store.Flush()
	defer store.Close()

	data := []byte(`{"key":"value"}`)
	parserMap["testRoute"] = jsonParser
	err := store.AppendPayload("testRoute", data, "json", time.Now())
	assert.NoError(t, err)

	// JSON1 query to retrieve the item such as key = value
	payloads, err := store.ExecuteQuery("SELECT * FROM parsed_payloads WHERE data->>'key' = 'value'")
	assert.NoError(t, err)
	assert.Len(t, payloads, 1)
	assert.Equal(t, "testRoute", payloads[0]["route"])
	expected, err := json.Marshal(map[string]interface{}{"key": "value"})
	assert.NoError(t, err)
	assert.Equal(t, expected, payloads[0]["data"])

}
