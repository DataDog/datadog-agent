// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package opener

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidatedDirectIOAlignment(t *testing.T) {
	tests := []struct {
		name  string
		value uint32
		want  int
	}{
		{name: "zero falls back to default", value: 0, want: directIOAlignment},
		{name: "non power of two falls back", value: 3, want: directIOAlignment},
		{name: "non power of two large falls back", value: 4097, want: directIOAlignment},
		{name: "above max falls back", value: maxDirectIOAlignment * 2, want: directIOAlignment},
		{name: "valid small power of two", value: 512, want: 512},
		{name: "valid default power of two", value: directIOAlignment, want: directIOAlignment},
		{name: "valid max power of two", value: maxDirectIOAlignment, want: maxDirectIOAlignment},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, validatedDirectIOAlignment(tt.value))
		})
	}
}
