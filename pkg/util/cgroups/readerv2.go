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

// parseCgroups parses the cgroups from the cgroupRoot and returns a map of cgroup id to cgroup,
// plus a map of sub-cgroup inodes to container IDs. The sub-cgroup inode map allows DogStatsD
// origin detection to resolve containers whose cgroup namespace root is a sub-directory of the
// registered cgroup path (e.g. CRI-O's .scope/container/ layout on cgroupv2).
func (r *readerV2) parseCgroups() (map[string]Cgroup, map[uint64]string, error) {
	res := make(map[string]Cgroup)
	subInodes := make(map[uint64]string)

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
			// If we already have a cgroup with this id, that means that we have a sub-cgroup.
			// In that case, we keep the parent's stats path, but also record the sub-cgroup's
			// inode so DogStatsD can find the container if a client reports it.
			if _, exists := res[id]; !exists {
				relPath, err := filepath.Rel(r.cgroupRoot, fullPath)
				if err != nil {
					return err
				}
				res[id] = newCgroupV2(id, r.cgroupRoot, relPath, r.cgroupControllers, r.pidMapper)
			} else {
				if subInode := inodeForPath(fullPath); subInode != unknownInode {
					subInodes[subInode] = id
				}
			}
		}

		return err
	})
	return res, subInodes, err
}

func readCgroupControllers(cgroupRoot string) (map[string]struct{}, error) {
	controllersMap := make(map[string]struct{})
	err := parseFile(defaultFileReader, path.Join(cgroupRoot, controllersFile), func(s string) error {
		controllers := strings.FieldsSeq(s)
		for c := range controllers {
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
