// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package mount holds mount related files
package mount

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// ResolverInterface defines the resolver interface
type ResolverInterface interface {
	IsMountIDValid(mountID uint32) (bool, error)
	SyncCache() error
	Delete(mountID uint32, mountIDUnique uint64) error
	ResolveFilesystem(pathKey model.PathKey, pid uint32) (string, error)
	Insert(m model.Mount) error
	InsertMoved(m model.Mount) error
	ResolveMountRoot(pathKey model.PathKey, pid uint32) (string, model.MountSource, model.MountOrigin, error)
	ResolveMountPath(pathKey model.PathKey, pid uint32) (string, model.MountSource, model.MountOrigin, error)
	ResolveMount(pathKey model.PathKey, pid uint32) (*model.Mount, model.MountSource, model.MountOrigin, error)
	SendStats() error
	ToJSON() ([]byte, error)
	Iterate(cb func(*model.Mount))
}
