// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package lookup provides a lookup table for the protocol package.
package lookup

// This generates each lookup table's implementation in ./luts.go:
// - Use /var/tmp/datadog-agent/system-probe/go-toolchains
//   as the location for the Go toolchains to be downloaded to.
//   Each toolchain version is around 500 MiB on disk.
//go:generate go run --tags linux_bpf ./internal/generate_luts.go --test-program ./internal/testprogram/program.go --package lookup --out ./luts.go --min-go 1.13 --arch amd64,arm64 --shared-build-dir /var/tmp/datadog-agent/system-probe/go-toolchains
