// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package procmgrdecision

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRecordValid(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{name: "enabled with newline", content: "v1 enabled\n", want: true},
		{name: "enabled without newline", content: "v1 enabled", want: true},
		{name: "disabled with newline", content: "v1 disabled\n", want: false},
		{name: "disabled without newline", content: "v1 disabled", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRecord([]byte(tt.content))
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseRecordInvalid(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "empty", content: ""},
		{name: "only newline", content: "\n"},
		{name: "version only", content: "v1\n"},
		{name: "other version", content: "v2 enabled\n"},
		{name: "other value", content: "v1 maybe\n"},
		{name: "extra token", content: "v1 enabled extra\n"},
		{name: "two lines", content: "v1 enabled\nv1 enabled\n"},
		{name: "double trailing newline", content: "v1 enabled\n\n"},
		{name: "leading space", content: " v1 enabled\n"},
		{name: "trailing space", content: "v1 enabled \n"},
		{name: "crlf", content: "v1 enabled\r\n"},
		{name: "uppercase", content: "V1 ENABLED\n"},
		{name: "65 bytes", content: strings.Repeat("x", 65)},
		{name: "65 bytes that look valid", content: "v1 enabled" + strings.Repeat(" ", 54) + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseRecord([]byte(tt.content))
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalid)
		})
	}
}

// TestNilStoreIsNotFound checks that a typed nil store passed as RecordReader reads as no record,
// so the resolver falls through to the seed or the default without panicking.
func TestNilStoreIsNotFound(t *testing.T) {
	var s *Store
	enabled, found, err := s.Read()
	require.NoError(t, err)
	assert.False(t, enabled)
	assert.False(t, found)

	var rec RecordReader = s
	d, err := Resolve(nil, rec, nil)
	require.NoError(t, err)
	assert.Equal(t, Decision{Enabled: Default, Source: SourceDefault}, d)
}
