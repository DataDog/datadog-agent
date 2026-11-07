// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config embeds the KMT platform definitions (platforms.json) and
// exposes the per-platform environment variables defined there to the test
// binaries.
package config

import (
	_ "embed" // embed platforms.json
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
)

//go:embed platforms.json
var platformsJSON []byte

type platform struct {
	Kernel      string          `json:"kernel"`
	TestEnvVars map[string]bool `json:"test_env_vars,omitempty"`
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

// TestEnvVars returns the test environment variables of the KMT platform
// entry with the given platform key (e.g. "amazon_5.10") under the given
// architecture ("x86_64" or "arm64"), as defined in platforms.json. The key is
// the platforms.json map key, not the kernel version. It returns an error if
// no entry matches, so that a misidentified platform fails loudly instead of
// silently skipping tests.
func TestEnvVars(arch, platformKey string) (map[string]string, error) {
	plats, err := parsePlatforms()
	if err != nil {
		return nil, err
	}

	byArch := plats.X8664
	if arch == "arm64" {
		byArch = plats.Arm64
	}

	plat, ok := byArch[platformKey]
	if !ok {
		return nil, fmt.Errorf("no KMT platform %q under arch %q in platforms.json", platformKey, arch)
	}

	envVars := make(map[string]string, len(plat.TestEnvVars))
	for name, value := range plat.TestEnvVars {
		envVars[name] = strconv.FormatBool(value)
	}
	return envVars, nil
}
