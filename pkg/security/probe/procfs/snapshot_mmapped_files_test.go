// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package procfs

import (
	"os"
	"slices"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetMmapedFilesTimes(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	fi, err := os.Stat(exe)
	require.NoError(t, err)
	st := fi.Sys().(*syscall.Stat_t)

	files, err := GetMmapedFiles(uint32(os.Getpid()))
	require.NoError(t, err)
	i := slices.IndexFunc(files, func(f MmapedFile) bool { return f.Path == exe })
	require.NotEqual(t, -1, i, "%s is missing from %v", exe, files)
	assert.Equal(t, uint64(st.Ctim.Nano()), files[i].CTime)
	assert.Equal(t, uint64(st.Mtim.Nano()), files[i].MTime)
}
