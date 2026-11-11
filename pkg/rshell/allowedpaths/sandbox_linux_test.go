// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package allowedpaths

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenRegularRejectsProcPortalsOutsideAllowedPaths(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "descriptor-target")
	require.NoError(t, err)
	defer file.Close()

	sb, _, err := New([]string{"/proc"})
	require.NoError(t, err)
	defer sb.Close()

	paths := []string{
		fmt.Sprintf("/proc/self/fd/%d", file.Fd()),
		"/proc/self/root/etc/passwd",
	}
	for _, path := range paths {
		handle, err := sb.OpenRegular(path, "/")
		assert.Nil(t, handle, path)
		assert.Error(t, err, path)
	}
}
