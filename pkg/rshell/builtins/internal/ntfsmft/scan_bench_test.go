// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package ntfsmft

import (
	"context"
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

// BenchmarkScan scans a real NTFS volume (default C:\, override with
// RSHELL_NTFSDU_BENCH_TARGET) so CPU/memory profiles reflect a full $MFT walk.
// Run one iteration for profiling:
//
//	go test -run '^$' -bench '^BenchmarkScan$' -benchtime=1x \
//	    -cpuprofile cpu.prof -memprofile mem.prof ./builtins/internal/ntfsmft/
//
// Skips when raw MFT access is unavailable (non-admin, container filesystem, or
// a non-NTFS volume), so it is inert in CI.
func BenchmarkScan(b *testing.B) {
	target := `C:\`
	if t := os.Getenv("RSHELL_NTFSDU_BENCH_TARGET"); t != "" {
		target = t
	}

	// Cheap pre-check: open the volume without walking the $MFT so the skip
	// path costs no full scan and does not pollute the profile.
	h, _, err := openVolume(target[:1])
	if err != nil {
		if isRawMFTUnsupported(err) {
			b.Skipf("raw MFT access unavailable: %v", err)
		}
		b.Fatalf("openVolume(%s): %v", target[:1], err)
	}
	windows.CloseHandle(h)

	opts := Options{TopFiles: 10, TopExtensions: 10, TreeDepth: 1}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Scan(context.Background(), target, opts); err != nil {
			b.Fatalf("Scan: %v", err)
		}
	}
}
