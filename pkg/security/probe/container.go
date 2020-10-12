// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// ContainerResolver is used to resolve the container context of the events
type ContainerResolver struct{}

// GetContainerID returns the container id of the given pid
func (cr *ContainerResolver) GetContainerID(pid uint32) (*utils.ContainerID, error) {
	// Parse /proc/[pid]/moutinfo
	containerID, err := utils.GetProcContainerID(pid, pid)
	if err != nil {
		pErr, ok := err.(*os.PathError)
		if !ok {
			return nil, err
		}
		return nil, pErr
	}
	return &containerID, nil
}

// ResolveLabels resolves the label of a container from its container ID
func (cr *ContainerResolver) ResolveLabels(containerID string) ([]string, error) {
	// Do not use the tagger for now
	return []string{}, nil
}
