// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"os"
	"testing"

	legacyprocess "github.com/DataDog/gopsutil/process"
	"github.com/stretchr/testify/assert"
)

func TestSnapshotMemoryMappedFiles(t *testing.T) {
	pid := os.Getpid()

	// gopsutil
	fakeprocess := legacyprocess.Process{Pid: int32(pid)}
	smapsPtr, err := fakeprocess.MemoryMaps(false)
	if err != nil {
		t.Fatal(err)
	}
	if smapsPtr == nil {
		t.Fatal("nil smaps")
	}

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
			ok:   false,
		},
	}

	for _, entry := range entries {
		t.Run(entry.name, func(t *testing.T) {
			path, ok := extractPathFromMapsLine([]byte(entry.line))
			if ok != entry.ok {
				t.Errorf("expected ok=%t, got %t", entry.ok, ok)
			}
			if string(path) != entry.path {
				t.Errorf("expected %s, got %s", entry.path, path)
			}
		})
	}
}
