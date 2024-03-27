// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package mount holds mount related files
package mount

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// NoOpResolver returns an empty resolver
type NoOpResolver struct {
}

// IsMountIDValid returns whether the mountID is valid
func (mr *NoOpResolver) IsMountIDValid(mountID uint32) (bool, error) {
	return false, nil
}

// SyncCache Snapshots the current mount points of the system by reading through /proc/[pid]/mountinfo.
func (n *NoOpResolver) SyncCache(pid uint32) error {
	return nil
}

// Delete a mount from the cache
func (n *NoOpResolver) Delete(mountID uint32) error {
	return nil
}

// ResolveFilesystem returns the name of the filesystem
func (n *NoOpResolver) ResolveFilesystem(mountID uint32, device uint32, pid uint32, containerID string) (string, error) {
	return "", nil
}

// Insert a new mount point in the cache
func (n *NoOpResolver) Insert(m model.Mount, pid uint32) error {
	return nil
}

// DelPid removes the pid form the pid mapping
func (n *NoOpResolver) DelPid(pid uint32) {}

// ResolveMountRoot returns the root of a mount identified by its mount ID.
func (n *NoOpResolver) ResolveMountRoot(mountID uint32, device uint32, pid uint32, containerID string) (string, error) {
	return "", nil
}

// ResolveMountPath returns the path of a mount identified by its mount ID.
func (n *NoOpResolver) ResolveMountPath(mountID uint32, device uint32, pid uint32, containerID string) (string, error) {
	return "", nil
}

// ResolveMount returns the mount
func (n *NoOpResolver) ResolveMount(mountID uint32, device uint32, pid uint32, containerID string) (*model.Mount, error) {
	return nil, errors.New("not available")
}

// SendStats sends metrics about the current state of the mount resolver
func (n *NoOpResolver) SendStats() error {
	return nil
}
