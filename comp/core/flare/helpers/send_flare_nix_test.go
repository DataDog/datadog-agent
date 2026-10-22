// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package helpers

import (
	"os"
	"strconv"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assertArchiveClosed(t *testing.T, path string) {
	t.Helper()
	var archive syscall.Stat_t
	require.NoError(t, syscall.Stat(path, &archive))

	dir, err := os.Open("/dev/fd")
	require.NoError(t, err)
	fds, err := dir.Readdirnames(-1)
	dir.Close()
	require.NoError(t, err)

	for _, name := range fds {
		fd, err := strconv.Atoi(name)
		require.NoError(t, err)
		var st syscall.Stat_t
		if syscall.Fstat(fd, &st) == nil && st.Dev == archive.Dev && st.Ino == archive.Ino {
			assert.Failf(t, "the archive should be closed once the upload is over", "descriptor %d is still open on %s", fd, path)
		}
	}
}
