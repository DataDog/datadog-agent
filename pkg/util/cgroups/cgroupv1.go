// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroups

import (
	"os"
	"path/filepath"
	"syscall"
)

type cgroupV1 struct {
	identifier     string
	mountPoints    map[string]string
	path           string
	fr             fileReader
	pidMapper      pidMapper
	inode          uint64
	baseController string
}

func newCgroupV1(identifier, path, baseController string, mountPoints map[string]string, pidMapper pidMapper) *cgroupV1 {
	return &cgroupV1{
		identifier:     identifier,
		mountPoints:    mountPoints,
		path:           path,
		pidMapper:      pidMapper,
		fr:             defaultFileReader,
		baseController: baseController,
	}
}

func (c *cgroupV1) Identifier() string {
	return c.identifier
}

func (c *cgroupV1) Inode() uint64 {
	if c.inode > 2 {
		return c.inode
	}

	stat, err := os.Stat(c.pathFor(c.baseController, ""))
	if err != nil {
		return unknownInode
	}
	c.inode = stat.Sys().(*syscall.Stat_t).Ino
	if c.inode > 2 {
		return c.inode
	}
	return unknownInode
}

func (c *cgroupV1) GetParent() (Cgroup, error) {
	parentPath := filepath.Join(c.path, "/..")
	return newCgroupV1(filepath.Base(parentPath), parentPath, c.baseController, c.mountPoints, c.pidMapper), nil
}

func (c *cgroupV1) controllerMounted(controller string) bool {
	_, found := c.mountPoints[controller]
	return found
}

// Expects controller to exist, see controllerMounted
func (c *cgroupV1) pathFor(controller, file string) string {
	return filepath.Join(c.mountPoints[controller], c.path, file)
}
