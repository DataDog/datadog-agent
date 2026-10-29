// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package truncate

import (
	"math"
	"testing"
)

func TestParseSize(t *testing.T) {
	tests := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		// Bare integers
		{"0", 0, false},
		{"1", 1, false},
		{"1024", 1024, false},
		{"9223372036854775807", math.MaxInt64, false}, // MaxInt64
		{"9223372036854775808", 0, true},              // MaxInt64+1 → overflow

		// Binary suffixes — K/M/G/T case-insensitive
		{"1K", 1024, false},
		{"1k", 1024, false},
		{"1KiB", 1024, false},
		{"1kiB", 1024, false},
		{"2M", 2 * (1 << 20), false},
		{"2m", 2 * (1 << 20), false},
		{"1G", 1 << 30, false},
		{"1g", 1 << 30, false},
		{"1T", 1 << 40, false},
		{"1t", 1 << 40, false},
		{"1TiB", 1 << 40, false},
		{"1tiB", 1 << 40, false},

		// P and E: uppercase-only leading letter
		{"1P", 1 << 50, false},
		{"1PiB", 1 << 50, false},
		{"1E", 1 << 60, false},
		{"1EiB", 1 << 60, false},
		{"1p", 0, true}, // lowercase p rejected by GNU
		{"1e", 0, true}, // lowercase e rejected by GNU
		{"1piB", 0, true},
		{"1eiB", 0, true},

		// Decimal suffixes
		{"1KB", 1000, false},
		{"1kB", 1000, false},
		{"1MB", 1000 * 1000, false},
		{"1mB", 1000 * 1000, false},
		{"1GB", 1000 * 1000 * 1000, false},
		{"1gB", 1000 * 1000 * 1000, false},
		{"1TB", 1000 * 1000 * 1000 * 1000, false},
		{"1tB", 1000 * 1000 * 1000 * 1000, false},
		{"1PB", 1000 * 1000 * 1000 * 1000 * 1000, false},
		{"1EB", 1000 * 1000 * 1000 * 1000 * 1000 * 1000, false},

		// Case-sensitive trailing: "B" must be uppercase
		{"1kb", 0, true},
		{"1KIB", 0, true},
		{"1Kib", 0, true},

		// Relative-size prefixes — rejected with dedicated error
		{"+1K", 0, true},
		{"-1K", 0, true},
		{"<1K", 0, true},
		{">1K", 0, true},
		{"/1K", 0, true},
		{"%1K", 0, true},

		// Overflow via multiplier
		{"8E", 0, true}, // 8 * (1<<60) = 2^63 > MaxInt64
		{"9TB", 9 * 1000 * 1000 * 1000 * 1000, false}, // 9e12 < MaxInt64

		// T suffix: 2^40 = 1099511627776; MaxInt64/2^40 ≈ 8388607.99
		{"8192T", 8192 * (1 << 40), false}, // 2^53 < MaxInt64
		{"8388608T", 0, true},              // 2^63 exactly — overflows
		{"9223373TB", 0, true},             // 9223373 * 1e12 > MaxInt64

		// Malformed inputs
		{"", 0, true},
		{"abc", 0, true},
		{"1.5K", 0, true},
		{"1 K", 0, true},
		{" 1K", 0, true},
		{"1KB2", 0, true},
		{"Z", 0, true},
		{"1Z", 0, true},
		{"1Y", 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := parseSize(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Errorf("parseSize(%q) = %d, want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Errorf("parseSize(%q) unexpected error: %v", tc.in, err)
				return
			}
			if got != tc.want {
				t.Errorf("parseSize(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseSizeRelativeErrorMessage(t *testing.T) {
	_, err := parseSize("+100")
	if err == nil {
		t.Fatal("expected error for relative size")
	}
	if err != errRelativeSize {
		t.Errorf("expected errRelativeSize, got %v", err)
	}
}
