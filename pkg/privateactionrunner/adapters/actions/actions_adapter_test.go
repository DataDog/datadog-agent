// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package actions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetRootBundle(t *testing.T) {
	tests := []struct {
		name     string
		bundleID string
		expected string
	}{
		{
			name:     "returns three-part bundle unchanged",
			bundleID: "com.datadoghq.http",
			expected: "com.datadoghq.http",
		},
		{
			name:     "returns root of nested bundle",
			bundleID: "com.datadoghq.authoredscripts.helm",
			expected: "com.datadoghq.authoredscripts",
		},
		{
			name:     "returns short bundle unchanged",
			bundleID: "com.datadoghq",
			expected: "com.datadoghq",
		},
		{
			name:     "returns empty bundle unchanged",
			bundleID: "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, GetRootBundle(tt.bundleID))
		})
	}
}
