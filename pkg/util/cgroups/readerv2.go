// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroups

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
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

// parseCgroups parses the cgroups from the cgroupRoot and returns a map of
// cgroup id to cgroup. The provided map will only be populated with discovered
// cgroups, nothing stale.
func (r *readerV2) parseCgroups(res map[string]Cgroup) (map[string]Cgroup, error) {
	// Mark all existing cgroups for deletion. Those that are found in the DFS
	// will be retained ultimately in this map, the rest will be removed. This
	// allows us to elide expensive calls to newCgroupV2 -- one obligatory
	// syscall per call, string allocation per -- for cgroups we have already
	// seen.
	for id := range res {
		if cg, ok := res[id].(*cgroupV2); ok {
			cg.markedForDeletion = true
		}
	}

	// Avoid the use of filepath.WalkDir which is allocation hungry, at least as
	// of Go 1.23.8. Instead we perform our own depth-first search.
	stack := []string{r.cgroupRoot}

	for len(stack) > 0 {
		// Pop the directory to search from the stack, alter said stack.
		dir := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		id, err := r.filter(dir, filepath.Base(dir))
		if err != nil {
			// If SkipDir is returned, record the cgroup if valid, then skip
			// pushing subdirectories.
			if errors.Is(err, filepath.SkipDir) {
				if id != "" {
					relPath, err := filepath.Rel(r.cgroupRoot, dir)
					if err != nil {
						return nil, err
					}
					// Reuse existing cgroup if available
					if existing, ok := res[id]; ok {
						if cg, ok := existing.(*cgroupV2); ok {
							cg.relativePath = relPath
							cg.markedForDeletion = false
						}
					} else {
						res[id] = newCgroupV2(id, r.cgroupRoot, relPath, r.cgroupControllers, r.pidMapper)
					}
				}
				continue
			}
			return nil, err
		}
		if id != "" {
			relPath, err := filepath.Rel(r.cgroupRoot, dir)
			if err != nil {
				return nil, err
			}
			// Reuse existing cgroup if available
			if existing, ok := res[id]; ok {
				if cg, ok := existing.(*cgroupV2); ok {
					cg.relativePath = relPath
					cg.markedForDeletion = false
				}
			} else {
				res[id] = newCgroupV2(id, r.cgroupRoot, relPath, r.cgroupControllers, r.pidMapper)
			}
		}

		// Now read all entries and push only directories onto the stack. Note
		// this is allocation hungry itself. A direct call to unix.Open is
		// feasible but would require us to handle C-style strings.
		//
		// Might be worthwhile eventually.
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				subPath := filepath.Join(dir, entry.Name())
				stack = append(stack, subPath)
			}
		}
	}

	// Remove cgroups that are still marked for deletion
	for id, cg := range res {
		if cgv2, ok := cg.(*cgroupV2); ok && cgv2.markedForDeletion {
			delete(res, id)
		}
	}

	return res, nil
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
