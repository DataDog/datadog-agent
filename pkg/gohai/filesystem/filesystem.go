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

type FileSystem struct{}

type MountInfo struct {
	Name      string `json:"name"`
	SizeKB    uint64 `json:"kb_size"`
	MountedOn string `json:"mounted_on"`
}

var (
	timeout = 2 * time.Second
	ErrTimeoutExceeded = errors.New("timeout exceeded")
)

const name = "filesystem"

func (fs *FileSystem) Name() string {
	return name
}

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

func Get() ([]MountInfo, error) {
	return getWithTimeout(timeout)
}

func getWithTimeout(timeout time.Duration) ([]MountInfo, error) {
	mountInfoChan := make(chan []MountInfo, 1)
	errChan := make(chan error, 1)
	timeoutChan := time.After(timeout)

	go func() {
		mountInfo, err := getFileSystemInfo()
		if err == nil {
			mountInfoChan <- mountInfo
		} else {
			errChan <- err
		}
	}()

	select {
	case mountInfo := <-mountInfoChan:
		return mountInfo, nil
	case err := <-errChan:
		return nil, err
	case <-timeoutChan:
		return nil, ErrTimeoutExceeded
	}
}
