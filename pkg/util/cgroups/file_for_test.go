// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroups

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Return paths that look like Kubernetes cgroups
func containerCgroupKubePod(systemd bool) string {
	cID, err := randToken(32)
	if err != nil {
		panic("unable to get random data")
	}

	if systemd {
		return fmt.Sprintf("kubepods.slice/kubepods-besteffort.slice/kubepods-besteffort-podb3922967_14e1_4867_9388_461bac94b37e.slice/crio-%s.scope", cID)
	}
	return fmt.Sprintf("kubepods/kubepods-besteffort/kubepods-besteffort-podb3922967_14e1_4867_9388_461bac94b37e/%s", cID)
}

type memoryFile struct {
	strings.Reader
}

func (mf *memoryFile) Close() error {
	return nil
}

type cgroupMemoryFS struct {
	rootPath    string
	mountPoints map[string]string
	controllers map[string]struct{}
	files       map[string]string
}

func newCgroupMemoryFS(rootPath string) *cgroupMemoryFS {
	return &cgroupMemoryFS{
		rootPath:    rootPath,
		mountPoints: make(map[string]string),
		controllers: make(map[string]struct{}),
		files:       make(map[string]string),
	}
}

func (cfs *cgroupMemoryFS) enableControllers(controllers ...string) {
	for _, c := range controllers {
		cfs.mountPoints[c] = filepath.Join(cfs.rootPath, c)
		cfs.controllers[c] = struct{}{}
	}
}

// for cgroup v1 only
func (cfs *cgroupMemoryFS) setCgroupV1File(cg *cgroupV1, controller, name, content string) {
	if controllerPath, found := cfs.mountPoints[controller]; found {
		cfs.files[filepath.Join(controllerPath, cg.path, name)] = content
	}
}

// for cgroup v1 only
func (cfs *cgroupMemoryFS) deleteCgroupV1File(cg *cgroupV1, controller, name string) {
	if controllerPath, found := cfs.mountPoints[controller]; found {
		delete(cfs.files, filepath.Join(controllerPath, cg.path, name))
	}
}

// for cgroup v1 only
func (cfs *cgroupMemoryFS) createCgroupV1(id, path string) *cgroupV1 {
	cg := newCgroupV1(id, path, cfs.mountPoints, nil)
	cg.fr = cfs
	return cg
}

// for cgroup v2 only
func (cfs *cgroupMemoryFS) createCgroupV2(id, path string) *cgroupV2 {
	cg := newCgroupV2(id, cfs.rootPath, path, cfs.controllers, nil)
	cg.fr = cfs
	return cg
}

// for cgroup v2 only
func (cfs *cgroupMemoryFS) setCgroupV2File(cg *cgroupV2, name, content string) {
	cfs.files[filepath.Join(cg.cgroupRoot, cg.relativePath, name)] = content
}

// for cgroup v2 only
func (cfs *cgroupMemoryFS) deleteCgroupV2File(cg *cgroupV2, name string) {
	delete(cfs.files, filepath.Join(cg.cgroupRoot, cg.relativePath, name))
}

func (cfs *cgroupMemoryFS) open(path string) (file, error) {
	if content, found := cfs.files[path]; found {
		return &memoryFile{Reader: *strings.NewReader(content)}, nil
	}

	return nil, os.ErrNotExist
}
