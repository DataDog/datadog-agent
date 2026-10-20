// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package ntfsdu

import "testing"

func TestParseSize(t *testing.T) {
	cases := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"", 0, false},
		{"0", 0, false},
		{"100", 100, false},
		{"1K", 1024, false},
		{"1k", 1024, false},
		{"100M", 100 * 1024 * 1024, false},
		{"5G", 5 * 1024 * 1024 * 1024, false},
		{"2T", 2 * 1024 * 1024 * 1024 * 1024, false},
		{"10KB", 10 * 1024, false},
		{"10KiB", 10 * 1024, false},
		{"  4M  ", 4 * 1024 * 1024, false},
		{"10Q", 0, true},
		{"-5", 0, true},
		{"abc", 0, true},
		{"B", 0, true},
	}
	for _, c := range cases {
		got, err := parseSize(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseSize(%q): expected error, got %d", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseSize(%q): unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseSizeOverflow(t *testing.T) {
	// A value that overflows int64 when multiplied by the suffix must error,
	// not wrap around to a negative or truncated size.
	if _, err := parseSize("9999999999999T"); err == nil {
		t.Errorf("parseSize: expected overflow error for very large T value")
	}
}
