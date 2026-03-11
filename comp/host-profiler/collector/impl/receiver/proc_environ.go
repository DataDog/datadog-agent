// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package receiver

import (
	"bytes"
	"fmt"
	"os"
)

func readProcessEnv(pid int) (map[string]string, error) {
	path := fmt.Sprintf("/proc/%d/environ", pid)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}

	envMap := make(map[string]string)
	for _, entry := range bytes.Split(data, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		parts := bytes.SplitN(entry, []byte{'='}, 2)
		if len(parts) != 2 {
			continue
		}
		envMap[string(parts[0])] = string(parts[1])
	}

	return envMap, nil
}

func shouldProfileProcess(pid int, config MemoryProfilingConfig) bool {
	if config.Mode == "all" {
		return true
	}

	if config.Mode == "filtered" {
		envMap, err := readProcessEnv(pid)
		if err != nil {
			return false
		}
		value, exists := envMap[config.EnvVar]
		return exists && value == "1"
	}

	return false
}
