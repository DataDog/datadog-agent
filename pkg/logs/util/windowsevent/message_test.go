// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package windowsevent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasTruncatedFlag(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{
			name:    "empty message",
			message: "",
			want:    false,
		},
		{
			name:    "short message",
			message: "short",
			want:    false,
		},
		{
			name:    "normal message",
			message: "This is a normal log message",
			want:    false,
		},
		{
			name:    "message truncated at end",
			message: "This is a very long message that gets truncated...TRUNCATED...",
			want:    true,
		},
		{
			name:    "message truncated at beginning",
			message: "...TRUNCATED...This is a message that was truncated at the beginning",
			want:    true,
		},
		{
			name:    "message exactly truncated flag",
			message: "...TRUNCATED...",
			want:    true,
		},
		{
			name:    "message with truncated flag at both ends",
			message: "...TRUNCATED...content...TRUNCATED...",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasTruncatedFlag(tt.message)
			assert.Equal(t, tt.want, got)
		})
	}
}
