// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package preprocessor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
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
				rawMessage:      []byte(tc.rawMessage),
				label:           aggregate,
				labelAssignedBy: defaultLabelSource,
			}
			assert.Equal(t, tc.expectedResult, jsonDetector.ProcessAndContinue(messageContext))
			assert.Equal(t, tc.expectedLabel, messageContext.label)
		})
	}
}

func TestJSONLogsProcessedTelemetry(t *testing.T) {
	jsonDetector := NewJSONDetector()
	initial := metrics.TlmJSONLogsProcessed.WithValues().Get()

	// JSON message increments the counter
	ctx := &messageContext{rawMessage: []byte(`{"key":"value"}`), label: aggregate, labelAssignedBy: defaultLabelSource}
	jsonDetector.ProcessAndContinue(ctx)
	assert.Equal(t, initial+1, metrics.TlmJSONLogsProcessed.WithValues().Get())

	// Plain text does not increment
	ctx = &messageContext{rawMessage: []byte(`Not JSON`), label: aggregate, labelAssignedBy: defaultLabelSource}
	jsonDetector.ProcessAndContinue(ctx)
	assert.Equal(t, initial+1, metrics.TlmJSONLogsProcessed.WithValues().Get())

	// Already-labeled message does not increment (even if JSON)
	ctx = &messageContext{rawMessage: []byte(`{"key":"value"}`), label: aggregate, labelAssignedBy: "user_sample"}
	jsonDetector.ProcessAndContinue(ctx)
	assert.Equal(t, initial+1, metrics.TlmJSONLogsProcessed.WithValues().Get())
}

func TestJsonDetectorDoesntOverrideAssignedLabel(t *testing.T) {
	jsonDetector := NewJSONDetector()
	messageContext := &messageContext{
		rawMessage:      []byte(`{"key": "value"}`),
		label:           aggregate,
		labelAssignedBy: "Not default!",
	}
	assert.Equal(t, true, jsonDetector.ProcessAndContinue(messageContext))
	assert.Equal(t, aggregate, messageContext.label)
}
