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

type cgroupV2 struct {
	identifier   string
	cgroupRoot   string
	relativePath string
	controllers  map[string]struct{}
	fr           fileReader
	pidMapper    pidMapper
	inode        uint64
}

func newCgroupV2(identifier, cgroupRoot, relativePath string, controllers map[string]struct{}, inode uint64, pidMapper pidMapper) *cgroupV2 {
	return &cgroupV2{
		identifier:   identifier,
		cgroupRoot:   cgroupRoot,
		relativePath: relativePath,
		controllers:  controllers,
		pidMapper:    pidMapper,
		fr:           defaultFileReader,
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
	stat, err := os.Stat(parentPath)
	if err != nil {
		return nil, err
	}
	inode := stat.Sys().(*syscall.Stat_t).Ino
	return newCgroupV2(filepath.Base(parentPath), c.cgroupRoot, parentPath, c.controllers, inode, c.pidMapper), nil
}

func (c *cgroupV2) controllerActivated(controller string) bool {
	_, found := c.controllers[controller]
	return found
}

func (c *cgroupV2) pathFor(filename string) string {
	return filepath.Join(c.cgroupRoot, c.relativePath, filename)
}
