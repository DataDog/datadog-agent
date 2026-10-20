// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cgo

package zstd

import (
	"bytes"
	"testing"

	kzstd "github.com/klauspost/compress/zstd"
)

// FuzzCrossCompatibility asserts that data compressed by the CGO backend can
// be decompressed by the pure-Go (klauspost) backend and vice versa, for all
// inputs. This is the property that lets a single build interoperate with
// payloads produced by either backend.
func FuzzCrossCompatibility(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte("a"))
	f.Add([]byte(string(make([]byte, 1000))))
	f.Add([]byte("The quick brown fox jumps over the lazy dog"))
	f.Add(bytes.Repeat([]byte("abcd"), 250))

	// Pure-Go backend, configured with WithZeroFrames to match the production
	// nocgo backend (see zstd_nocgo.go).
	nocgoEnc, err := kzstd.NewWriter(nil,
		kzstd.WithEncoderLevel(kzstd.EncoderLevelFromZstd(1)),
		kzstd.WithZeroFrames(true),
	)
	if err != nil {
		f.Fatal(err)
	}
	nocgoDec, err := kzstd.NewReader(nil)
	if err != nil {
		f.Fatal(err)
	}
	defer nocgoDec.Close()

	f.Fuzz(func(t *testing.T, data []byte) {
		// CGO compress -> pure-Go decompress
		cgoCompressed, err := CompressLevel(nil, data, 1)
		if err != nil {
			t.Fatalf("CGO Compress failed: %v", err)
		}
		nocgoDecompressed, err := nocgoDec.DecodeAll(cgoCompressed, nil)
		if err != nil {
			t.Fatalf("pure-Go Decompress of CGO-compressed data failed: %v", err)
		}
		if !bytes.Equal(data, nocgoDecompressed) {
			t.Errorf("CGO -> pure-Go cross-compatibility failed")
		}

		// pure-Go compress -> CGO decompress
		nocgoCompressed := nocgoEnc.EncodeAll(data, nil)
		cgoDecompressed, err := Decompress(nil, nocgoCompressed)
		if err != nil {
			t.Fatalf("CGO Decompress of pure-Go-compressed data failed: %v", err)
		}
		if !bytes.Equal(data, cgoDecompressed) {
			t.Errorf("pure-Go -> CGO cross-compatibility failed")
		}
	})
}
