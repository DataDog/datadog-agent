// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

// Package filesystem returns information about available filesystems.
package filesystem

import (
	"errors"
	"strconv"
	"time"
)

// FileSystem implements the Collector interface, providing information about mounted filesystems.
type FileSystem struct{}

// MountInfo represents a mounted filesystem.
type MountInfo struct {
	// Name is the name of the mounted filesystem.
	Name string `json:"name"`
	// SizeKB is the size of the mounted filesystem in KB.
	SizeKB uint64 `json:"kb_size"`
	// MountedOn is the mount point path of the mounted filesystem.
	MountedOn string `json:"mounted_on"`
}

var (
	timeout = 2 * time.Second
	// ErrTimeoutExceeded represents a timeout error
	ErrTimeoutExceeded = errors.New("timeout exceeded")
)

const name = "filesystem"

// Name returns the name of the package
func (fs *FileSystem) Name() string {
	return name
}

// Collect returns the list of mounted filesystems as an object which can be used to generate a JSON
func (fs *FileSystem) Collect() (interface{}, error) {
	mounts, err := Get()
	if err != nil {
		return nil, err
	}

	results := make([]interface{}, len(mounts))
	for idx, mount := range mounts {
		tmpMount := mount
		results[idx] = map[string]string{
			"name":       tmpMount.Name,
			"kb_size":    strconv.FormatUint(tmpMount.SizeKB, 10),
			"mounted_on": tmpMount.MountedOn,
		}
	}

	return results, nil
}

// Get returns the list of mounted filesystems
func Get() ([]MountInfo, error) {
	return getWithTimeout(timeout, getFileSystemInfo)
}

// getWithTimeout is an internal helper for test purpose
func getWithTimeout(timeout time.Duration, getFileSystemInfo func() ([]MountInfo, error)) ([]MountInfo, error) {
	type infoRes struct {
		data []MountInfo
		err  error
	}

	mountInfoChan := make(chan infoRes, 1)
	go func() {
		mountInfo, err := getFileSystemInfo()
		mountInfoChan <- infoRes{
			data: mountInfo,
			err:  err,
		}
	}()

	select {
	case info := <-mountInfoChan:
		return info.data, info.err
	case <-time.After(timeout):
		return nil, ErrTimeoutExceeded
	}
}
