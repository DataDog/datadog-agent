// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package gooffsets provides the Go runtime struct offsets used to read Go
// pprof labels, per Go version and architecture.
package gooffsets

// This generates GetGoRuntimeOffsets's implementation in ./goruntime_offsets.go:
// - Use /var/tmp/datadog-agent/system-probe/go-toolchains
//   as the location for the Go toolchains to be downloaded to.
//go:generate go run ./internal/generate_goruntime_offsets_lut.go --test-program ./internal/testprogram/program.go --package gooffsets --out ./goruntime_offsets.go --min-go 1.13 --arch amd64,arm64 --shared-build-dir /var/tmp/datadog-agent/system-probe/go-toolchains

// A zero Hmap* offset means the Go version uses Swiss tables (go1.24+).
type GoRuntimeOffsets struct {
	// MOffset is the offset of "m" in "runtime.g".
	MOffset uint32
	// MGsignal is the offset of "gsignal" in "runtime.m".
	MGsignal uint32
	// Curg is the offset of "curg" in "runtime.m".
	Curg uint32
	// Labels is the offset of "labels" in "runtime.g".
	Labels uint32
	// HmapCount is the offset of "count" in "runtime.hmap".
	HmapCount uint32
	// HmapLog2BucketCount is the offset of "B" in "runtime.hmap".
	HmapLog2BucketCount uint32
	// HmapBuckets is the offset of "buckets" in "runtime.hmap".
	HmapBuckets uint32
}
