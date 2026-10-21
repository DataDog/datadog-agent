// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/probe/procfs"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestSnapshotMemoryMappedFiles(t *testing.T) {
	pid := os.Getpid()

	// gopsutil
	fakeprocess, err := process.NewProcess(int32(pid))
	if err != nil {
		t.Fatal(err)
	}
	smapsPtr, err := fakeprocess.MemoryMaps(false)
	if err != nil {
		t.Fatal(err)
	}
	if smapsPtr == nil {
		t.Fatal("nil smaps")
	}

	seenPaths := make(map[string]struct{})
	var gopsutilFiles []string
	for _, smap := range *smapsPtr {
		if len(gopsutilFiles) == MaxMmapedFiles {
			break
		}
		if len(smap.Path) == 0 {
			continue
		}
		if smap.Path[0] == '[' {
			continue
		}
		if _, seen := seenPaths[smap.Path]; seen {
			continue
		}
		seenPaths[smap.Path] = struct{}{}
		gopsutilFiles = append(gopsutilFiles, smap.Path)
	}

	// hand-made version
	ownImplemFiles, err := getMemoryMappedFiles(int32(pid), "")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, gopsutilFiles, ownImplemFiles)
}

func TestExtractPathFromMapsLine(t *testing.T) {
	entries := []struct {
		name string
		line string
		path string
		ok   bool
	}{
		{
			name: "stack",
			line: "fffe33c3000-ffffe33e4000 rw-p 00000000 00:00 0                          [stack]",
			path: "[stack]",
			ok:   true,
		},
		{
			name: "regular",
			line: "e1cc8924f000-e1cc89251000 rw-p 00030000 fd:00 6259                       /usr/lib/aarch64-linux-gnu/ld-linux-aarch64.so.1",
			path: "/usr/lib/aarch64-linux-gnu/ld-linux-aarch64.so.1",
			ok:   true,
		},
		{
			name: "regular with space",
			line: "e1cc8924f000-e1cc89251000 rw-p 00030000 fd:00 6259                       /usr/lib/aarch64-linux-gnu/ld linux aarch64.so.1",
			path: "/usr/lib/aarch64-linux-gnu/ld linux aarch64.so.1",
			ok:   true,
		},
		{
			name: "anonymous",
			line: "7398749de000-739874a00000 rw-p 00000000 00:00 0",
			path: "",
			ok:   true,
		},
	}

	for _, entry := range entries {
		t.Run(entry.name, func(t *testing.T) {
			mapsEntry, ok := procfs.ParseMapsLine([]byte(entry.line))
			if ok != entry.ok {
				t.Errorf("expected ok=%t, got %t", entry.ok, ok)
			}
			if mapsEntry.Pathname != entry.path {
				t.Errorf("expected %s, got %s", entry.path, mapsEntry.Pathname)
			}
		})
	}
}

func TestAddFilesTimes(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	file := filepath.Join(dir, "lib.so")
	require.NoError(t, os.WriteFile(file, nil, 0o644))
	mtime := time.Date(2024, 5, 6, 7, 8, 9, 123456789, time.UTC)
	require.NoError(t, os.Chtimes(file, mtime, mtime))
	var st unix.Stat_t
	require.NoError(t, unix.Stat(file, &st))

	pn := &ProcessNode{}
	pn.Process.Pid = uint32(os.Getpid())
	pn.addFiles([]string{file}, NewActivityTreeNodeStats(), model.NewFakeEvent, nil, "")

	var leaf *FileNode
	children := pn.Files
	for _, name := range strings.Split(file[1:], "/") {
		leaf = children[name]
		require.NotNil(t, leaf, "%s is missing from the tree", name)
		children = leaf.Children
	}
	require.NotNil(t, leaf.File)
	assert.Equal(t, uint64(st.Ctim.Nano()), leaf.File.CTime)
	assert.Equal(t, uint64(st.Mtim.Nano()), leaf.File.MTime)
}
