// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	_ "embed"
	"sort"
	"testing"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/assert"
)

//go:embed fixtures/log_bytes
var logData []byte

func TestLogAggregator(t *testing.T) {
	t.Run("parseLogPayload should return empty log array on empty data", func(t *testing.T) {
		checks, err := ParseLogPayload(api.Payload{Data: []byte(""), Encoding: encodingEmpty})
		assert.NoError(t, err)
		assert.Empty(t, checks)
	})

	t.Run("parseLogPayload should return empty log array on empty json object", func(t *testing.T) {
		checks, err := ParseLogPayload(api.Payload{Data: []byte("{}"), Encoding: encodingJSON})
		assert.NoError(t, err)
		assert.Empty(t, checks)
	})

	t.Run("parseLogPayload should return valid checks on valid ", func(t *testing.T) {
		logs, err := ParseLogPayload(api.Payload{Data: logData, Encoding: encodingGzip})
		assert.NoError(t, err)
		assert.Equal(t, 1, len(logs))
		assert.Equal(t, "callme", logs[0].name())
		expectedTags := []string{"singer:adele"}
		sort.Strings(expectedTags)
		gotTags := logs[0].GetTags()
		sort.Strings(gotTags)
		assert.Equal(t, expectedTags, gotTags)
	})
}
