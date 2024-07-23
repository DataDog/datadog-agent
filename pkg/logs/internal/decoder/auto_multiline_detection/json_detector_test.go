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
		rawMessage    string
		expectedLabel Label
	}{
		{`{"key": "value"}`, noAggregate},
		{`    {"key": "value"}`, noAggregate},
		{`    { "key": "value"}`, noAggregate},
		{`    {."key": "value"}`, aggregate},
		{`.{"key": "value"}`, aggregate},
		{`{"another_key": "another_value"}`, noAggregate},
		{`{"key": 12345}`, noAggregate},
		{`{"array": [1,2,3]}`, noAggregate},
		{`not json`, aggregate},
		{`{foo}`, aggregate},
		{`{bar"}`, aggregate},
		{`"FOO"}`, aggregate},
	}

	for _, tc := range testCases {
		t.Run(string(tc.rawMessage), func(t *testing.T) {
			messageContext := &messageContext{
				rawMessage: []byte(tc.rawMessage),
				label:      aggregate,
			}
			jsonDetector.Process(messageContext)
			assert.Equal(t, tc.expectedLabel, messageContext.label)
		})
	}
}
