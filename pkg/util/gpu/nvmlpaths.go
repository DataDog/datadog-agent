// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package gpu

import (
	"os"
	"path/filepath"
)

// isContainerized checks if the agent is running in a containerized environment. We can't use the env.IsContainerized() function here
// to avoid a circular dependency.
func isContainerized() bool {
	return os.Getenv("DOCKER_DD_AGENT") == "1"
}

// GenerateDefaultNvmlPaths generates the default paths for the NVML library, where it's commonly installed,
// taking into account containerized environments and the HOST_ROOT environment variable.
func GenerateDefaultNvmlPaths() []string {
	systemPaths := []string{
		"/usr/lib/x86_64-linux-gnu/libnvidia-ml.so.1",                   // default system install
		"/run/nvidia/driver/usr/lib/x86_64-linux-gnu/libnvidia-ml.so.1", // nvidia-gpu-operator install
	}

	hostRoot := os.Getenv("HOST_ROOT")
	if hostRoot == "" {
		if isContainerized() {
			hostRoot = "/host" // default host root for containerized environments without HOST_ROOT set
		} else {
			// no host root variable and not containerized, assume we are running on the bare host
			return systemPaths
		}
	}

	var containerizedPaths []string
	for _, path := range systemPaths {
		containerizedPaths = append(containerizedPaths, filepath.Join(hostRoot, path))
	}
	return containerizedPaths
}
