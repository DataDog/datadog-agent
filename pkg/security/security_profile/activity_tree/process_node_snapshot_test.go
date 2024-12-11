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
		gopsutilFiles = append(gopsutilFiles, smap.Path)
	}

	// hand-made version
	ownImplemFiles, err := getMemoryMappedFiles(int32(pid), "")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, gopsutilFiles, ownImplemFiles)
}
