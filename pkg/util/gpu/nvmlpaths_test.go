// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package gpu

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateDefaultLibraryPaths(t *testing.T) {
	t.Run("non containerized", func(t *testing.T) {
		paths := GenerateDefaultNvmlPaths()
		for _, path := range paths {
			require.NotContains(t, path, "/host")
			require.True(t, strings.HasSuffix(path, "/libnvidia-ml.so.1"))
			require.True(t, strings.HasPrefix(path, "/usr") || strings.HasPrefix(path, "/run/nvidia/driver/usr"), "invalid prefix for path: %s", path)
		}
	})

	t.Run("containerized, no HOST_ROOT", func(t *testing.T) {
		t.Setenv("DOCKER_DD_AGENT", "1")
		t.Setenv("HOST_ROOT", "")
		paths := GenerateDefaultNvmlPaths()
		for _, path := range paths {
			require.True(t, strings.HasPrefix(path, "/host"), "path %s does not start with /host", path)
			require.True(t, strings.HasSuffix(path, "/libnvidia-ml.so.1"))
		}
	})

	t.Run("kubernetes, with HOST_ROOT", func(t *testing.T) {
		// IsContainerized() only returns true if DOCKER_DD_AGENT is set. In K8s the detection is done based on HOST_ROOT
		t.Setenv("HOST_ROOT", "/k8s-root")
		paths := GenerateDefaultNvmlPaths()
		for _, path := range paths {
			require.True(t, strings.HasPrefix(path, "/k8s-root"), "path %s does not start with /k8s-root", path)
			require.True(t, strings.HasSuffix(path, "/libnvidia-ml.so.1"))
		}
	})

	t.Run("containerized, with HOST_ROOT", func(t *testing.T) {
		t.Setenv("DOCKER_DD_AGENT", "1")
		t.Setenv("HOST_ROOT", "/something-else")
		paths := GenerateDefaultNvmlPaths()
		for _, path := range paths {
			require.True(t, strings.HasPrefix(path, "/something-else"), "path %s does not start with /something-else", path)
			require.True(t, strings.HasSuffix(path, "/libnvidia-ml.so.1"))
		}
	})
}
