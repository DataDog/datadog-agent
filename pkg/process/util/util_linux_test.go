// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package util

import (
	"os"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetRootNSPID(t *testing.T) {
	t.Run("HOST_PROC not set", func(t *testing.T) {
		pid, err := GetRootNSPID()
		assert.Nil(t, err)
		assert.Equal(t, os.Getpid(), pid)
	})

	t.Run("HOST_PROC set but not available", func(t *testing.T) {
		t.Setenv("HOST_PROC", "/foo/bar")
		pid, err := GetRootNSPID()
		assert.NotNil(t, err)
		assert.Equal(t, 0, pid)
	})
}

func oldWithAllProcs(procRoot string, fn func(int) error) error {
	files, err := os.ReadDir(procRoot)
	if err != nil {
		return err
	}

	for _, f := range files {
		if !f.IsDir() || f.Name() == "." || f.Name() == ".." {
			continue
		}

		var pid int
		if pid, err = strconv.Atoi(f.Name()); err != nil {
			continue
		}

		if err = fn(pid); err != nil {
			return err
		}
	}
	return nil
}

func BenchmarkOldWithAllProcs(b *testing.B) {

	var pids []int
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pids := []int{}
		oldWithAllProcs("/proc", func(pid int) error {
			pids = append(pids, pid)
			return nil
		})
	}
	runtime.KeepAlive(pids)
}

func BenchmarkWithAllProcs(b *testing.B) {
	var pids []int

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pids = []int{}
		WithAllProcs("/proc", func(pid int) error {
			pids = append(pids, pid)
			return nil
		})
	}
	runtime.KeepAlive(pids)
}

func BenchmarkAllPidsProcs(b *testing.B) {
	var pids []int

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pids, _ = AllPidsProcs("/proc")
	}
	runtime.KeepAlive(pids)
}
