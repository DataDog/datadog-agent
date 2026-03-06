// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package event

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetEventPriorityFromString(t *testing.T) {
	p, err := GetEventPriorityFromString("normal")
	require.NoError(t, err)
	assert.Equal(t, PriorityNormal, p)

	p, err = GetEventPriorityFromString("low")
	require.NoError(t, err)
	assert.Equal(t, PriorityLow, p)

	_, err = GetEventPriorityFromString("invalid")
	assert.Error(t, err)
}

func TestGetAlertTypeFromString(t *testing.T) {
	tests := []struct {
		input    string
		expected AlertType
	}{
		{"error", AlertTypeError},
		{"warning", AlertTypeWarning},
		{"info", AlertTypeInfo},
		{"success", AlertTypeSuccess},
	}
	for _, tc := range tests {
		at, err := GetAlertTypeFromString(tc.input)
		require.NoError(t, err)
		assert.Equal(t, tc.expected, at)
	}

	// invalid returns AlertTypeInfo as default with an error
	at, err := GetAlertTypeFromString("invalid")
	assert.Error(t, err)
	assert.Equal(t, AlertTypeInfo, at)
}

func TestEventString(t *testing.T) {
	e := &Event{
		Title:     "test event",
		Text:      "something happened",
		Ts:        1234567890,
		Priority:  PriorityNormal,
		Host:      "myhost",
		AlertType: AlertTypeError,
		Tags:      []string{"env:prod"},
	}
	s := e.String()
	assert.Contains(t, s, `"msg_title":"test event"`)
	assert.Contains(t, s, `"host":"myhost"`)
	assert.Contains(t, s, `"alert_type":"error"`)
}
