// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package goid provides a function to get the current goroutine ID.
package goid

// This generates GetGoroutineIDOffset's implementation in ./goid_offset.go:
// - Use /var/tmp/datadog-agent/system-probe/go-toolchains
//   as the location for the Go toolchains to be downloaded to.
//   Each toolchain version is around 500 MiB on disk.
//go:generate go run ./internal/generate_goid_lut.go --test-program ./internal/testprogram/program.go --package goid --out ./goid_offset.go --min-go 1.13 --arch amd64,arm64 --shared-build-dir /var/tmp/datadog-agent/system-probe/go-toolchains
