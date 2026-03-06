// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"
	"time"

	"github.com/mailru/easyjson/jwriter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEasyjsonTime(t *testing.T) {
	now := time.Now()
	ejt := NewEasyjsonTime(now)
	assert.Equal(t, now, ejt.GetInnerTime())
}

func TestNewEasyjsonTimeIfNotZero(t *testing.T) {
	t.Run("non-zero time", func(t *testing.T) {
		now := time.Now()
		result := NewEasyjsonTimeIfNotZero(now)
		require.NotNil(t, result)
		assert.Equal(t, now, result.GetInnerTime())
	})

	t.Run("zero time", func(t *testing.T) {
		var zeroTime time.Time
		result := NewEasyjsonTimeIfNotZero(zeroTime)
		assert.Nil(t, result)
	})
}

func TestEasyjsonTime_MarshalEasyJSON(t *testing.T) {
	t.Run("valid time", func(t *testing.T) {
		// Use a fixed time for predictable output
		fixedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
		ejt := NewEasyjsonTime(fixedTime)

		w := &jwriter.Writer{}
		ejt.MarshalEasyJSON(w)

		data, err := w.BuildBytes()
		require.NoError(t, err)
		assert.Equal(t, `"2024-01-15T10:30:00Z"`, string(data))
	})

	t.Run("time with nanoseconds", func(t *testing.T) {
		fixedTime := time.Date(2024, 1, 15, 10, 30, 0, 123456789, time.UTC)
		ejt := NewEasyjsonTime(fixedTime)

		w := &jwriter.Writer{}
		ejt.MarshalEasyJSON(w)

		data, err := w.BuildBytes()
		require.NoError(t, err)
		assert.Contains(t, string(data), "2024-01-15T10:30:00.123456789Z")
	})

	t.Run("year outside valid range", func(t *testing.T) {
		// Year -1 is outside [0, 9999]
		invalidTime := time.Date(-1, 1, 1, 0, 0, 0, 0, time.UTC)
		ejt := NewEasyjsonTime(invalidTime)

		w := &jwriter.Writer{}
		ejt.MarshalEasyJSON(w)

		assert.Error(t, w.Error)
		assert.Contains(t, w.Error.Error(), "year outside of range")
	})

	t.Run("year 10000 outside valid range", func(t *testing.T) {
		invalidTime := time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
		ejt := NewEasyjsonTime(invalidTime)

		w := &jwriter.Writer{}
		ejt.MarshalEasyJSON(w)

		assert.Error(t, w.Error)
	})
}

func TestEasyjsonTime_UnmarshalJSON(t *testing.T) {
	t.Run("valid JSON time", func(t *testing.T) {
		ejt := &EasyjsonTime{}
		err := ejt.UnmarshalJSON([]byte(`"2024-01-15T10:30:00Z"`))
		require.NoError(t, err)

		expected := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
		assert.Equal(t, expected, ejt.GetInnerTime())
	})

	t.Run("invalid JSON time", func(t *testing.T) {
		ejt := &EasyjsonTime{}
		err := ejt.UnmarshalJSON([]byte(`"not a time"`))
		assert.Error(t, err)
	})
}

func TestEasyjsonTime_GetInnerTime(t *testing.T) {
	now := time.Now()
	ejt := NewEasyjsonTime(now)
	assert.Equal(t, now, ejt.GetInnerTime())
}

// mockMarshaler implements easyjson.Marshaler for testing
type mockMarshaler struct {
	data []string
}

func (m *mockMarshaler) MarshalEasyJSON(w *jwriter.Writer) {
	w.RawByte('[')
	for i, s := range m.data {
		if i > 0 {
			w.RawByte(',')
		}
		w.String(s)
	}
	w.RawByte(']')
}

func TestMarshalEasyJSON(t *testing.T) {
	t.Run("with data", func(t *testing.T) {
		m := &mockMarshaler{data: []string{"a", "b", "c"}}
		result, err := MarshalEasyJSON(m)
		require.NoError(t, err)
		assert.Equal(t, `["a","b","c"]`, string(result))
	})

	t.Run("empty data", func(t *testing.T) {
		m := &mockMarshaler{data: []string{}}
		result, err := MarshalEasyJSON(m)
		require.NoError(t, err)
		assert.Equal(t, `[]`, string(result))
	})
}
