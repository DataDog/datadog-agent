// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroups

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	controllersFile = "cgroup.controllers"
)

type readerV2 struct {
	cgroupRoot        string
	cgroupControllers map[string]struct{}
	filter            ReaderFilter
	pidMapper         pidMapper
}

func newReaderV2(procPath, cgroupRoot string, filter ReaderFilter, pidMapperID string) (*readerV2, error) {
	controllers, err := readCgroupControllers(cgroupRoot)
	if err != nil {
		return nil, err
	}

	return &readerV2{
		cgroupRoot:        cgroupRoot,
		cgroupControllers: controllers,
		filter:            filter,
		pidMapper:         getPidMapper(procPath, cgroupRoot, "", filter, pidMapperID),
	}, nil
}

// parseCgroups parses the cgroups from the cgroupRoot and returns a map of cgroup id to cgroup.
func (r *readerV2) parseCgroups() (map[string]Cgroup, error) {
	res := make(map[string]Cgroup)

	err := filepath.WalkDir(r.cgroupRoot, func(fullPath string, de fs.DirEntry, err error) error {
		if err != nil {
			// if the error is a permission issue skip the directory
			if errors.Is(err, fs.ErrPermission) {
				log.Debugf("skipping %s due to permission error", fullPath)
				return filepath.SkipDir
			}
			return err
		}
		if !de.IsDir() {
			return nil
		}

		id, err := r.filter(fullPath, de.Name())
		if id != "" {
			relPath, err := filepath.Rel(r.cgroupRoot, fullPath)
			if err != nil {
				return err
			}
			res[id] = newCgroupV2(id, r.cgroupRoot, relPath, r.cgroupControllers, r.pidMapper)
		}

		return err
	})
	return res, err
}

func readCgroupControllers(cgroupRoot string) (map[string]struct{}, error) {
	controllersMap := make(map[string]struct{})
	err := parseFile(defaultFileReader, path.Join(cgroupRoot, controllersFile), func(s string) error {
		controllers := strings.Fields(s)
		for _, c := range controllers {
			controllersMap[c] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(controllersMap) == 0 {
		return nil, fmt.Errorf("no cgroup controllers activated at: %s", path.Join(cgroupRoot, controllersFile))
	}

	return controllersMap, nil
}
