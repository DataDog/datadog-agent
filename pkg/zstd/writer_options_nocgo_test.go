// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !cgo

package zstd

import (
	"bytes"
	"io"
	"testing"

	kzstd "github.com/klauspost/compress/zstd"
)

func TestWriterOptions(t *testing.T) {
	t.Setenv("ZSTD_NOCGO_WINDOW", "3")
	if w, err := NewWriterLevel(io.Discard, BestSpeed); err == nil {
		w.Close()
		t.Fatal("invalid window accepted")
	}
	for _, data := range [][]byte{nil, bytes.Repeat([]byte("symbols"), 200000)} {
		var compressed bytes.Buffer
		w, err := NewWriterLevelWithOptions(&compressed, BestSpeed,
			WithWindowSize(3), WithWindowSize(1<<20), WithLowerEncoderMem(false))
		if err != nil {
			t.Fatal(err)
		}
		// Low-memory mode cannot change on reset, so false must already be set.
		if err := w.(*kzstd.Encoder).ResetWithOptions(&compressed, kzstd.WithLowerEncoderMem(false)); err != nil {
			t.Fatalf("low-memory mode was not disabled: %v", err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		decoded, err := Decompress(nil, compressed.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, decoded) {
			t.Fatal("round trip changed payload")
		}
		if len(data) > 0 {
			var header kzstd.Header
			if err := header.Decode(compressed.Bytes()); err != nil {
				t.Fatal(err)
			}
			if header.WindowSize != 1<<20 {
				t.Fatalf("window=%d, want 1 MiB", header.WindowSize)
			}
		}
	}
}
