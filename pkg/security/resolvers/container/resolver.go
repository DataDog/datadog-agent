// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package container holds container related files
package container

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// Resolver is used to resolve the container context of the events
type Resolver struct {
	fs *utils.CGroupFS
}

// New creates a new container resolver
func New() *Resolver {
	return &Resolver{
		fs: utils.DefaultCGroupFS(),
	}
}

// GetContainerContext returns the container id, cgroup context, and cgroup sysfs path of the given pid
func (cr *Resolver) GetContainerContext(pid uint32) (containerutils.ContainerID, model.CGroupContext, string, error) {
	// Parse /proc/[pid]/task/[pid]/cgroup and /sys/fs/cgroup/[cgroup]
	id, ctx, path, err := cr.fs.FindCGroupContext(pid, pid)
	if err != nil {
		return "", model.CGroupContext{}, "", err
	}

	return id, model.CGroupContext{
		CGroupID: ctx.CGroupID,
		CGroupFile: model.PathKey{
			Inode:   ctx.CGroupFileInode,
			MountID: ctx.CGroupFileMountID,
		},
	}, path, nil
}
