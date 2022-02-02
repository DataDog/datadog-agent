// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package cgroups

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

var sampleCgroupProcs = `1142219
1142238
1142208
1142129`

func TestCgroupProcsPidMapper(t *testing.T) {
	cfs := newCgroupMemoryFS("/test/fs/cgroup")
	cfs.enableControllers(defaultBaseController)

	cgFooV1 := cfs.createCgroupV1("foov1", containerCgroupKubePod(false))
	cgFooV1.pidMapper = &cgroupProcsPidMapper{
		fr: cfs,
		cgroupProcsFilePathBuilder: func(relativeCgroupPath string) string {
			return filepath.Join(cfs.rootPath, defaultBaseController, relativeCgroupPath, cgroupProcsFile)
		},
	}
	cfs.setCgroupV1File(cgFooV1, defaultBaseController, cgroupProcsFile, sampleCgroupProcs)

	cgFooV2 := cfs.createCgroupV2("foov2", containerCgroupKubePod(false))
	cgFooV2.pidMapper = &cgroupProcsPidMapper{
		fr: cfs,
		cgroupProcsFilePathBuilder: func(relativeCgroupPath string) string {
			return filepath.Join(cfs.rootPath, "", relativeCgroupPath, cgroupProcsFile)
		},
	}
	cfs.setCgroupV2File(cgFooV2, cgroupProcsFile, sampleCgroupProcs)

	pids, err := cgFooV1.GetPIDs(0)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []int{1142219, 1142238, 1142208, 1142129}, pids)

	pids, err = cgFooV2.GetPIDs(0)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []int{1142219, 1142238, 1142208, 1142129}, pids)
}

func TestProcPidMapper(t *testing.T) {
	fakeFsPath := t.TempDir()
	paths := []string{
		"proc/420",
		"proc/421",
		"proc/430",
	}

	for _, p := range paths {
		finalPath := filepath.Join(fakeFsPath, p)
		assert.NoErrorf(t, os.MkdirAll(finalPath, 0o750), "impossible to create temp directory '%s'", finalPath)
	}

	cgFooV1 := cgroupV1{
		path: "a/b/c/foo1",
		pidMapper: &procPidMapper{
			fr:               defaultFileReader,
			readerFilter:     DefaultFilter,
			procPath:         filepath.Join(fakeFsPath, "/proc"),
			cgroupController: defaultBaseController,
		},
	}
	assert.NoError(t, ioutil.WriteFile(filepath.Join(fakeFsPath, "/proc/420/cgroup"), []byte("12:memory:/a/b/c/foo1\n11:devices:/a/b/c/fooWRONG\n10:hugetlb:/"), 0o640))
	assert.NoError(t, ioutil.WriteFile(filepath.Join(fakeFsPath, "/proc/421/cgroup"), []byte("12:memory:/a/b/c/foo1\n10:hugetlb:/"), 0o640))

	cgFooV2 := cgroupV2{
		relativePath: "a/b/c/foo3",
		pidMapper: &procPidMapper{
			fr:               defaultFileReader,
			readerFilter:     DefaultFilter,
			procPath:         filepath.Join(fakeFsPath, "/proc"),
			cgroupController: "",
		},
	}
	assert.NoError(t, ioutil.WriteFile(filepath.Join(fakeFsPath, "/proc/430/cgroup"), []byte("0::/a/b/c/foo3"), 0o640))

	pids, err := cgFooV1.GetPIDs(0)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []int{420, 421}, pids)

	pids, err = cgFooV2.GetPIDs(0)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []int{430}, pids)
}
