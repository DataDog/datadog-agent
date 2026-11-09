// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gcp

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateLabelValue(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "short value is unchanged",
			in:   "ci-init-123-e2e-foo",
			want: "ci-init-123-e2e-foo",
		},
		{
			name: "value at the limit is unchanged",
			in:   strings.Repeat("a", MaxResourceLabelValueLen),
			want: strings.Repeat("a", MaxResourceLabelValueLen),
		},
		{
			name: "ascii value over the limit is truncated to the limit",
			in:   strings.Repeat("a", MaxResourceLabelValueLen+10),
			want: strings.Repeat("a", MaxResourceLabelValueLen),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := TruncateLabelValue(tc.in); got != tc.want {
				t.Fatalf("TruncateLabelValue(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestTruncateLabelValueRuneBoundary ensures a multi-byte rune straddling the
// byte limit is dropped entirely instead of being cut in half, which would
// otherwise produce an invalid UTF-8 string that GCP rejects.
func TestTruncateLabelValueRuneBoundary(t *testing.T) {
	// "é" is 2 bytes in UTF-8, so a run of them crosses the limit on an
	// odd byte boundary, splitting the rune at MaxResourceLabelValueLen.
	in := strings.Repeat("é", MaxResourceLabelValueLen)

	got := TruncateLabelValue(in)

	if len(got) > MaxResourceLabelValueLen {
		t.Fatalf("result is %d bytes, want <= %d", len(got), MaxResourceLabelValueLen)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("result %q is not valid UTF-8", got)
	}
	// With a 63-byte budget and 2-byte runes, we keep 31 full runes (62 bytes).
	if want := strings.Repeat("é", MaxResourceLabelValueLen/2); got != want {
		t.Fatalf("TruncateLabelValue(%q) = %q, want %q", in, got, want)
	}
}
