// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package container

import (
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// Resolver is used to resolve the container context of the events
type Resolver struct{}

// GetContainerID returns the container id of the given pid
func (cr *Resolver) GetContainerID(pid uint32) (utils.ContainerID, error) {
	// Parse /proc/[pid]/task/[pid]/cgroup
	return utils.GetProcContainerID(pid, pid)
}
