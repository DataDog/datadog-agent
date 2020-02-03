// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package cgroup

import (
	"io/ioutil"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func setupFS(pidFdsMap map[int][]int) (*tempFolder, map[int]string, error) {
	f, err := newTempFolder("")
	if err != nil {
		return nil, nil, err
	}

	// Make map that stores paths to file descriptors files
	pidPathMap := make(map[int]string)

	// Add fd files to path <root>/<pid>/fd/
	// Note: this file path is not exact because TempDir adds a random hash to each layer
	for pid, fds := range pidFdsMap {
		g, err := ioutil.TempDir(f.RootPath, strconv.Itoa(pid))
		if err != nil {
			return nil, nil, err
		}
		p, err := ioutil.TempDir(g, "fd")
		if err != nil {
			return nil, nil, err
		}

		pidPathMap[pid] = p

		for _, fd := range fds {
			_, err := ioutil.TempFile(p, strconv.Itoa(fd))
			if err != nil {
				return nil, nil, err
			}
		}
	}
	return f, pidPathMap, nil
}

func TestGetFileDescriptorLen(t *testing.T) {
	// Map of pids to file descriptors
	pidFdsMap := map[int][]int{12345: {1}, 23456: {1, 11}, 34567: {1, 11, 111}}
	f, pidPathMap, err := setupFS(pidFdsMap)
	assert.Nil(t, err)

	for pid, fds := range pidFdsMap {
		path := pidPathMap[pid]
		hostProcFunc = func(combineWith ...string) string { return path }
		result, err := GetFileDescriptorLen(pid)

		assert.Nil(t, err)
		assert.Equal(t, len(fds), result)
	}

	// Clean up temp dirs and files
	f.removeAll()
}
