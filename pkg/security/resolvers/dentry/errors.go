// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package dentry holds dentry related files
package dentry

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// ErrERPCRequestNotProcessed is used to notify that the eRPC request was not processed
type ErrERPCRequestNotProcessed struct {
	LeafKey model.PathKey
	PathKey model.PathKey
	Depth   int64
	Caller  Caller
}

func (err ErrERPCRequestNotProcessed) Error() string {
	if err.LeafKey == (model.PathKey{}) && err.PathKey == (model.PathKey{}) && err.Depth == 0 && err.Caller == "" {
		return "erpc_not_processed"
	}
	if err.Depth > 1 {
		return fmt.Sprintf("erpc_not_processed at depth=%d key=%d/%d/%d (leaf=%d/%d/%d, caller=%s)",
			err.Depth, err.PathKey.MountID, err.PathKey.Inode, err.PathKey.PathID,
			err.LeafKey.MountID, err.LeafKey.Inode, err.LeafKey.PathID, err.Caller)
	}
	return fmt.Sprintf("erpc_not_processed (leaf=%d/%d/%d, caller=%s)",
		err.LeafKey.MountID, err.LeafKey.Inode, err.LeafKey.PathID, err.Caller)
}

// ErrTruncatedParentsERPC is used to notify that some parents of the path are missing
type ErrTruncatedParentsERPC struct {
	LeafKey model.PathKey
	PathKey model.PathKey
	Depth   int64
	Caller  Caller
}

func (err ErrTruncatedParentsERPC) Error() string {
	if err.LeafKey == (model.PathKey{}) && err.PathKey == (model.PathKey{}) && err.Depth == 0 && err.Caller == "" {
		return "truncated_parents_erpc"
	}
	return fmt.Sprintf("truncated_parents_erpc at depth=%d key=%d/%d/%d (leaf=%d/%d/%d, caller=%s)",
		err.Depth, err.PathKey.MountID, err.PathKey.Inode, err.PathKey.PathID,
		err.LeafKey.MountID, err.LeafKey.Inode, err.LeafKey.PathID, err.Caller)
}

// ErrTruncatedParents is used to notify that some parents of the path are missing
type ErrTruncatedParents struct {
	LeafKey model.PathKey
	PathKey model.PathKey
	Depth   int64
	Caller  Caller
}

func (err ErrTruncatedParents) Error() string {
	if err.LeafKey == (model.PathKey{}) && err.PathKey == (model.PathKey{}) && err.Depth == 0 && err.Caller == "" {
		return "truncated_parents"
	}
	return fmt.Sprintf("truncated_parents at depth=%d key=%d/%d/%d (leaf=%d/%d/%d, caller=%s)",
		err.Depth, err.PathKey.MountID, err.PathKey.Inode, err.PathKey.PathID,
		err.LeafKey.MountID, err.LeafKey.Inode, err.LeafKey.PathID, err.Caller)
}

// ErrERPCResolution is used to notify that the eRPC resolution failed
type ErrERPCResolution struct {
	LeafKey model.PathKey
	PathKey model.PathKey
	Depth   int64
	Caller  Caller
}

func (err ErrERPCResolution) Error() string {
	if err.LeafKey == (model.PathKey{}) && err.PathKey == (model.PathKey{}) && err.Depth == 0 && err.Caller == "" {
		return "erpc_resolution"
	}
	return fmt.Sprintf("erpc_resolution at depth=%d key=%d/%d/%d (leaf=%d/%d/%d, caller=%s)",
		err.Depth, err.PathKey.MountID, err.PathKey.Inode, err.PathKey.PathID,
		err.LeafKey.MountID, err.LeafKey.Inode, err.LeafKey.PathID, err.Caller)
}

// ErrKernelMapResolution is used to notify that the Kernel maps resolution failed
type ErrKernelMapResolution struct {
	LeafKey model.PathKey
	PathKey model.PathKey
	Depth   int64
	Caller  Caller
}

func (err ErrKernelMapResolution) Error() string {
	if err.LeafKey == (model.PathKey{}) && err.PathKey == (model.PathKey{}) && err.Depth == 0 && err.Caller == "" {
		return "map_resolution"
	}
	return fmt.Sprintf("map_resolution at depth=%d key=%d/%d/%d (leaf=%d/%d/%d, caller=%s)",
		err.Depth, err.PathKey.MountID, err.PathKey.Inode, err.PathKey.PathID,
		err.LeafKey.MountID, err.LeafKey.Inode, err.LeafKey.PathID, err.Caller)
}

// ErrDentryPathKeyNotFound is used to notify that the request key is missing from the kernel maps
type ErrDentryPathKeyNotFound struct {
	LeafKey model.PathKey
	PathKey model.PathKey
	Depth   int64
	Caller  Caller
}

func (err ErrDentryPathKeyNotFound) Error() string {
	if err.LeafKey == (model.PathKey{}) && err.Depth == 0 && err.Caller == "" {
		return fmt.Sprintf("dentry path key not found %d/%d", err.PathKey.MountID, err.PathKey.Inode)
	}
	if err.Depth > 0 && err.LeafKey != err.PathKey {
		return fmt.Sprintf("dentry parent path key not found %d/%d/%d at depth=%d (leaf=%d/%d/%d, caller=%s)",
			err.PathKey.MountID, err.PathKey.Inode, err.PathKey.PathID, err.Depth,
			err.LeafKey.MountID, err.LeafKey.Inode, err.LeafKey.PathID, err.Caller)
	}
	return fmt.Sprintf("dentry path key not found %d/%d/%d (depth=%d, caller=%s)",
		err.PathKey.MountID, err.PathKey.Inode, err.PathKey.PathID, err.Depth, err.Caller)
}
