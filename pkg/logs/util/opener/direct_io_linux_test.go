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
		name      string
		value     uint32
		want      int
		wantError bool
	}{
		{name: "zero is unsupported", value: 0, wantError: true},
		{name: "non power of two is unsupported", value: 3, wantError: true},
		{name: "non power of two large is unsupported", value: 4097, wantError: true},
		{name: "above max is unsupported", value: maxDirectIOAlignment * 2, wantError: true},
		{name: "valid small power of two", value: 512, want: 512},
		{name: "valid default power of two", value: directIOAlignment, want: directIOAlignment},
		{name: "valid max power of two", value: maxDirectIOAlignment, want: maxDirectIOAlignment},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validatedDirectIOAlignment(tt.value)
			if tt.wantError {
				require.Error(t, err)
				require.Contains(t, err.Error(), "alignment")
				require.Zero(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
