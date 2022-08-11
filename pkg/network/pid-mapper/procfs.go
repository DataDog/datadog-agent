// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package mapper

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// WalkProcFds walks the fd directory of each entry in
// procfs. It executes handleFd for each entry in /proc/<pid/fd
func WalkProcFds(handleFd func(string) error) error {
	procRoot := util.HostProc()
	d, err := os.Open(procRoot)
	if err != nil {
		return err
	}
	defer d.Close()

	fnames, err := d.Readdirnames(-1)
	if err != nil {
		return err
	}

	for _, fname := range fnames {
		pid, err := strconv.ParseInt(fname, 10, 32)
		if err != nil {
			// if not numeric name, just skip
			continue
		}

		fdpath := filepath.Join(d.Name(), fname, "fd")
		err = walkFds(int32(pid), fdpath, handleFd)
		if err != nil {
			return err
		}
	}

	return nil
}

func walkFds(pid int32, path string, handleFd func(string) error) error {
	fddir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fddir.Close()

	fdnames, err := fddir.Readdirnames(-1)
	if err != nil {
		return err
	}

	for _, fdname := range fdnames {
		fdPath := filepath.Join(path, fdname)

		err = handleFd(fdPath)
		if err != nil {
			return fmt.Errorf("failed to process fd at %s: %w\n", fdPath, err)
		}
	}
	return nil
}
