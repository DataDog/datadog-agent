// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataTypeMarshalText(t *testing.T) {
	tests := []struct {
		dt   DataType
		want string
	}{
		{Gauge, "gauge"},
		{Count, "count"},
		{Rate, "rate"},
	}
	for _, tt := range tests {
		b, err := tt.dt.MarshalText()
		require.NoError(t, err)
		assert.Equal(t, tt.want, string(b))
	}
}

func TestDataTypeUnmarshalText(t *testing.T) {
	tests := []struct {
		text string
		want DataType
	}{
		{"gauge", Gauge},
		{"count", Count},
		{"rate", Rate},
	}
	for _, tt := range tests {
		var dt DataType
		err := dt.UnmarshalText([]byte(tt.text))
		require.NoError(t, err)
		assert.Equal(t, tt.want, dt)
	}
}

func TestDataTypeUnmarshalTextInvalid(t *testing.T) {
	var dt DataType
	err := dt.UnmarshalText([]byte("invalid"))
	assert.Error(t, err)
}

func TestDataTypeMarshalTextInvalid(t *testing.T) {
	dt := DataType(99)
	_, err := dt.MarshalText()
	assert.Error(t, err)
}
