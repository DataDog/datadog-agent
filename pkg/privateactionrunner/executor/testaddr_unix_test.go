// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package executor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// testListenAddr returns a Unix socket path short enough for macOS's ~104-byte sun_path limit.
func testListenAddr(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "par")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "e.sock")
}
