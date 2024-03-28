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
	SyncCache(pid uint32) error
	Delete(mountID uint32) error
	ResolveFilesystem(mountID uint32, device uint32, pid uint32, containerID string) (string, error)
	Insert(m model.Mount, pid uint32) error
	DelPid(pid uint32)
	ResolveMountRoot(mountID uint32, device uint32, pid uint32, containerID string) (string, error)
	ResolveMountPath(mountID uint32, device uint32, pid uint32, containerID string) (string, error)
	ResolveMount(mountID uint32, device uint32, pid uint32, containerID string) (*model.Mount, error)
	SendStats() error
}
