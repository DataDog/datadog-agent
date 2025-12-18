// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Process interface {
	Uids() ([]uint32, error)
	Gids() ([]uint32, error)
}

// ReadProcessFileLimit reads a file from a process's file system. It will
// resolve it using the provided *os.Root, and check that the file is properly
// accessible by the process.
func ReadProcessFileLimit(proc Process, root *os.Root, name string, maxFileSize int64) ([]byte, os.FileInfo, error) {
	name = filepath.Clean(name)
	if filepath.IsAbs(name) {
		// when receiving an absolute path, we consider it as relative to the root
		name = strings.TrimPrefix(name, string(os.PathSeparator))
	}
	uids, err := proc.Uids()
	if err != nil {
		return nil, nil, err
	}
	gids, err := proc.Gids()
	if err != nil {
		return nil, nil, err
	}
	if len(uids) < 2 || len(gids) < 2 {
		return nil, nil, errors.New("invalid process IDs")
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if fi.IsDir() {
		return nil, nil, fmt.Errorf("file %s is a directory", f.Name())
	}
	if !fi.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("file %s is not a regular file", f.Name())
	}
	isExecutable := fi.Mode().Perm()&0111 > 0
	if isExecutable {
		return nil, nil, fmt.Errorf("file %s is executable", f.Name())
	}
	if err := checkFilePermissions(fi, uids, gids); err != nil {
		return nil, nil, err
	}
	r := io.LimitReader(f, maxFileSize)
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	return b, fi, nil
}

func RootedGlob(hostroot string, glob string) []string {
	glob = filepath.Clean(glob)
	if !filepath.IsAbs(glob) && glob != "" {
		return nil
	}
	path := filepath.Join(hostroot, glob)
	matches, _ := filepath.Glob(path)
	var allMatches []string
	for _, m := range matches {
		if m, ok := strings.CutPrefix(m, hostroot); ok && m != "" {
			allMatches = append(allMatches, m)
		}
	}
	return allMatches
}
