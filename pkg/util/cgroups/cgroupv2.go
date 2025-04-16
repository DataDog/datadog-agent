// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroups

import (
	"path/filepath"
)

type cgroupV2 struct {
	// 8-byte aligned fields first
	controllers map[string]struct{}
	pidMapper   pidMapper
	fr          fileReader
	inode       uint64

	// string fields (16 bytes each on 64-bit systems)
	identifier   string
	cgroupRoot   string
	relativePath string

	// bool field last
	markedForDeletion bool
}

func newCgroupV2(identifier, cgroupRoot, relativePath string, controllers map[string]struct{}, pidMapper pidMapper) *cgroupV2 {
	return &cgroupV2{
		identifier:   identifier,
		cgroupRoot:   cgroupRoot,
		relativePath: relativePath,
		controllers:  controllers,
		pidMapper:    pidMapper,
		fr:           defaultFileReader,
		inode:        inodeForPath(filepath.Join(cgroupRoot, relativePath)),
	}
}

func (c *cgroupV2) Identifier() string {
	return c.identifier
}

func (c *cgroupV2) Inode() uint64 {
	return c.inode
}

func (c *cgroupV2) GetParent() (Cgroup, error) {
	parentPath := filepath.Join(c.relativePath, "/..")
	return newCgroupV2(filepath.Base(parentPath), c.cgroupRoot, parentPath, c.controllers, c.pidMapper), nil
}

func (c *cgroupV2) controllerActivated(controller string) bool {
	_, found := c.controllers[controller]
	return found
}

func (c *cgroupV2) pathFor(filename string) string {
	return filepath.Join(c.cgroupRoot, c.relativePath, filename)
}
