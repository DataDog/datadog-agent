// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package truncate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTruncateUmask on Windows just verifies the file is created; Windows
// does not have a Unix-style umask and os.Root uses the default ACL.
func TestTruncateUmask(t *testing.T) {
	dir := t.TempDir()
	_, _, code := truncateRun(t, "truncate -s 10 newfile.txt", dir)
	require.Equal(t, 0, code)
	info, err := os.Stat(filepath.Join(dir, "newfile.txt"))
	require.NoError(t, err)
	assert.Equal(t, int64(10), info.Size())
}
