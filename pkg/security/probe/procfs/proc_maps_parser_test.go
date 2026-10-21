// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package procfs

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadMappedFilesManyLibraries checks that a process loading hundreds of
// shared libraries, as Python data science stacks do, keeps all of them.
func TestReadMappedFilesManyLibraries(t *testing.T) {
	var maps strings.Builder
	var want []string
	for i := range 300 {
		path := fmt.Sprintf("/usr/lib/python3/dist-packages/ext%d.so", i)
		fmt.Fprintf(&maps, "7f%08x0000-7f%08x1000 r--p 00000000 08:02 %d %s\n", i, i, 1000+i, path)
		fmt.Fprintf(&maps, "7f%08x1000-7f%08x2000 r-xp 00001000 08:02 %d %s\n", i, i, 1000+i, path)
		want = append(want, path)
	}
	maps.WriteString("7ffd00000000-7ffd00002000 r-xp 00000000 00:00 0 [vdso]\n")

	got, err := readMappedFiles(strings.NewReader(maps.String()), MaxMmapedFilesPerProcess, FilterExecutableRegularFiles)
	require.NoError(t, err)
	assert.Equal(t, want, got)

	got, err = readMappedFiles(strings.NewReader(maps.String()), 10, FilterExecutableRegularFiles)
	require.NoError(t, err)
	assert.Equal(t, want[:10], got)
}
