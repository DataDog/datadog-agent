// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package selector

import (
	"testing"
)

// Asserts that an unrecognized compression kind falls back to noop.
// Derived from rule CompressorFallback in compression.allium.
func TestUnrecognizedKindFallsBackToNoop(t *testing.T) {
	for _, kind := range []string{"bogus", "lz4", "", "ZSTD"} {
		c := NewCompressor(kind, 0)
		if c == nil {
			t.Fatalf("NewCompressor(%q, 0) returned nil", kind)
		}
		if got := c.ContentEncoding(); got != "identity" {
			t.Errorf("NewCompressor(%q, 0).ContentEncoding() = %q, want %q (noop)", kind, got, "identity")
		}
	}
}
