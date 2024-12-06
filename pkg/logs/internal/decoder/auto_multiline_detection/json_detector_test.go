// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJsonDetector(t *testing.T) {
	jsonDetector := NewJSONDetector()
	testCases := []struct {
		rawMessage     string
		expectedLabel  Label
		expectedResult bool
	}{
		{`{"key": "value"}`, noAggregate, false},
		{`    {"key": "value"}`, noAggregate, false},
		{`    { "key": "value"}`, noAggregate, false},
		{`    {."key": "value"}`, aggregate, true},
		{`.{"key": "value"}`, aggregate, true},
		{`{"another_key": "another_value"}`, noAggregate, false},
		{`{"key": 12345}`, noAggregate, false},
		{`{"array": [1,2,3]}`, noAggregate, false},
		{`not json`, aggregate, true},
		{`{foo}`, aggregate, true},
		{`{bar"}`, aggregate, true},
		{`"FOO"}`, aggregate, true},
		{`{}`, noAggregate, false},
		{` {}`, noAggregate, false},
		{` {    }`, noAggregate, false},
		{`{    }`, noAggregate, false},
	}

	for _, tc := range testCases {
		t.Run(string(tc.rawMessage), func(t *testing.T) {
			messageContext := &messageContext{
				rawMessage: []byte(tc.rawMessage),
				label:      aggregate,
			}
			assert.Equal(t, tc.expectedResult, jsonDetector.ProcessAndContinue(messageContext))
			assert.Equal(t, tc.expectedLabel, messageContext.label)
		})
	}
}
