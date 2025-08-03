// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:generate go run --tags linux_bpf,test ./internal/build.go --test-program ./internal/server.go --out ../testdata/builds --min-go 1.13 --arch "amd64,arm64" --shared-build-dir "/var/tmp/datadog-agent/system-probe/go-toolchains"

// Package precompiledserver holds scripts to generate static HTTPs servers in every golang version (from 1.13 to 1.25)
package precompiledserver
