// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config embeds the KMT platform definitions (platforms.json) and
// exposes the per-platform test flags defined there to the test binaries.
package config

import (
	_ "embed" // embed platforms.json
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// TestFlagEnvPrefix is prepended to every platforms.json test_flags key to turn
// it into the environment variable consumed by the test binaries. For example,
// the "network-tracer-fentry-supported" flag is exposed as
// DD_KMT_TEST_FLAG_NETWORK_TRACER_FENTRY_SUPPORTED.
//
// The test binaries reading these variables (e.g. pkg/ebpf/ebpftest) hardcode
// the resulting names; keep them in sync with platforms.json.
const TestFlagEnvPrefix = "DD_KMT_TEST_FLAG_"

//go:embed platforms.json
var platformsJSON []byte

type platform struct {
	Kernel    string          `json:"kernel"`
	TestFlags map[string]bool `json:"test_flags,omitempty"`
}

type platforms struct {
	X8664 map[string]platform `json:"x86_64"`
	Arm64 map[string]platform `json:"arm64"`
}

var (
	parseOnce    sync.Once
	parsed       *platforms
	parseInitErr error
)

func parsePlatforms() (*platforms, error) {
	parseOnce.Do(func() {
		parsed = &platforms{}
		if err := json.Unmarshal(platformsJSON, parsed); err != nil {
			parseInitErr = fmt.Errorf("unmarshalling embedded platforms.json: %w", err)
		}
	})
	return parsed, parseInitErr
}

// TestFlagEnvVars returns the test flags of the KMT platform matching the
// given architecture and kernel release (both as reported by uname) as a map
// of environment variable names to values. arch must be "x86_64" or "arm64".
// It returns an empty map if no platform matches (e.g. custom kernels).
func TestFlagEnvVars(arch, kernelRelease string) (map[string]string, error) {
	plats, err := parsePlatforms()
	if err != nil {
		return nil, err
	}

	byArch := plats.X8664
	if arch == "arm64" {
		byArch = plats.Arm64
	}

	for _, plat := range byArch {
		if plat.Kernel != kernelRelease {
			continue
		}

		envVars := make(map[string]string, len(plat.TestFlags))
		for flag, value := range plat.TestFlags {
			name := TestFlagEnvPrefix + strings.ToUpper(strings.ReplaceAll(flag, "-", "_"))
			envVars[name] = strconv.FormatBool(value)
		}
		return envVars, nil
	}

	return map[string]string{}, nil
}
