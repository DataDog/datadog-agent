// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kernel

import (
	"os"
	"strconv"
)

// AllPidsProcs will return all pids under procRoot
func AllPidsProcs(procRoot string) ([]int, error) {
	f, err := os.Open(procRoot)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dirs, err := f.Readdirnames(-1)
	if err != nil {
		return nil, err
	}

	pids := make([]int, 0, len(dirs))
	for _, name := range dirs {
		if pid, err := strconv.Atoi(name); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

// WithAllProcs will execute `fn` for every pid under procRoot. `fn` is
// passed the `pid`. If `fn` returns an error the iteration aborts,
// returning the last error returned from `fn`.
func WithAllProcs(procRoot string, fn func(int) error) error {
	pids, err := AllPidsProcs(procRoot)
	if err != nil {
		return err
	}

	for _, pid := range pids {
		if err = fn(pid); err != nil {
			return err
		}
	}
	return nil
}
