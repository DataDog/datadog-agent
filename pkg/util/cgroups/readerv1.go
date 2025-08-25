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
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultBaseController = "memory"
)

type readerV1 struct {
	mountPoints    map[string]string
	cgroupRoot     string
	filter         ReaderFilter
	pidMapper      pidMapper
	baseController string
}

func newReaderV1(procPath string, mountPoints map[string]string, baseController string, filter ReaderFilter, pidMapperID string) (*readerV1, error) {
	if baseController == "" {
		baseController = defaultBaseController
	}

	if path, found := mountPoints[baseController]; found {
		return &readerV1{
			mountPoints:    mountPoints,
			cgroupRoot:     path,
			filter:         filter,
			pidMapper:      getPidMapper(procPath, path, baseController, filter, pidMapperID),
			baseController: baseController,
		}, nil
	}

	return nil, &InvalidInputError{Desc: fmt.Sprintf("cannot create cgroup readerv1: %s controller not found", baseController)}
}

func (r *readerV1) parseCgroups() (map[string]Cgroup, error) {
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

			res[id] = newCgroupV1(id, relPath, r.baseController, r.mountPoints, r.pidMapper)
		}
		return err
	})

	return res, err
}
