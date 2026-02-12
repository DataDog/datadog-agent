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
func (mr *NoOpResolver) IsMountIDValid(_ uint32) (bool, error) {
	return false, nil
}

// SyncCache Snapshots the current mount points of the system by reading through /proc/.../mountinfo.
func (mr *NoOpResolver) SyncCache() error {
	return nil
}

// HasListMount returns true if the kernel has the listmount() syscall, false otherwise
func (mr *NoOpResolver) HasListMount() bool {
	return false
}

// SyncCacheFromListMount Snapshots the current mount points of the system by calling `listmount`
func (mr *NoOpResolver) SyncCacheFromListMount() error {
	return nil
}

// Delete a mount from the cache
func (mr *NoOpResolver) Delete(_ uint32, _ uint64) error {
	return nil
}

// ResolveFilesystem returns the name of the filesystem
func (mr *NoOpResolver) ResolveFilesystem(_ uint32, _ uint32) (string, error) {
	return "", nil
}

// Insert a new mount point in the cache
func (mr *NoOpResolver) Insert(_ model.Mount) error {
	return nil
}

// ResolveMountRoot returns the root of a mount identified by its mount ID.
func (mr *NoOpResolver) ResolveMountRoot(_ uint32, _ uint32) (string, model.MountSource, model.MountOrigin, error) {
	return "", model.MountSourceUnknown, model.MountOriginUnknown, nil
}

// ResolveMountPath returns the path of a mount identified by its mount ID.
func (mr *NoOpResolver) ResolveMountPath(_ uint32, _ uint32) (string, model.MountSource, model.MountOrigin, error) {
	return "", model.MountSourceUnknown, model.MountOriginUnknown, nil
}

// ResolveMount returns the mount
func (mr *NoOpResolver) ResolveMount(_ uint32, _ uint32) (*model.Mount, model.MountSource, model.MountOrigin, error) {
	return nil, model.MountSourceUnknown, model.MountOriginUnknown, errors.New("not available")
}

// SendStats sends metrics about the current state of the mount resolver
func (mr *NoOpResolver) SendStats() error {
	return nil
}

// ToJSON return a json version of the cache
func (mr *NoOpResolver) ToJSON() ([]byte, error) {
	return nil, nil
}

// InsertMoved inserts a mount from move_mount
func (mr *NoOpResolver) InsertMoved(_ model.Mount) error {
	return nil
}

// Iterate iterates over all the mounts in the cache and calls the callback function for each mount
func (mr *NoOpResolver) Iterate(_ func(*model.Mount)) {
}
