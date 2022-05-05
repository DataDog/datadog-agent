// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd
// +build containerd

package containerd

import (
	"errors"
	"fmt"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"

	"github.com/DataDog/datadog-agent/pkg/config"
	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/system"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// buildWorkloadMetaContainer generates a workloadmeta.Container from a containerd.Container
func buildWorkloadMetaContainer(container containerd.Container, containerdClient cutil.ContainerdItf) (workloadmeta.Container, error) {
	if container == nil {
		return workloadmeta.Container{}, fmt.Errorf("cannot build workloadmeta container from nil containerd container")
	}

	info, err := containerdClient.Info(container)
	if err != nil {
		return workloadmeta.Container{}, err
	}

	spec, err := containerdClient.Spec(container)
	if err != nil {
		return workloadmeta.Container{}, err
	}

	envs, err := containerdClient.EnvVars(container)
	if err != nil {
		return workloadmeta.Container{}, err
	}

	image, err := workloadmeta.NewContainerImage(info.Image)
	if err != nil {
		log.Debugf("cannot split image name %q: %s", info.Image, err)
	}

	status, err := containerdClient.Status(container)
	if err != nil {
		if !errdefs.IsNotFound(err) {
			return workloadmeta.Container{}, err
		}

		// The container exists, but there isn't a task associated to it. That
		// means that the container is not running, which is all we need to know
		// in this function (we can set any status != containerd.Running).
		status = containerd.Unknown
	}

	networkIPs := make(map[string]string)
	ip, err := extractIP(container, containerdClient)
	if err != nil {
		log.Debugf("cannot get IP of container %s", err)
	} else {
		networkIPs[""] = ip
	}

	// Some attributes in workloadmeta.Container cannot be fetched from
	// containerd. I've marked those as "Not available".
	return workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   container.ID(),
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:   "", // Not available
			Labels: info.Labels,
		},
		Image:   image,
		EnvVars: envs,
		Ports:   nil, // Not available
		Runtime: workloadmeta.ContainerRuntimeContainerd,
		State: workloadmeta.ContainerState{
			Running:    status == containerd.Running,
			Status:     extractStatus(status),
			CreatedAt:  info.CreatedAt,
			StartedAt:  info.CreatedAt, // StartedAt not available in containerd, mapped to CreatedAt
			FinishedAt: time.Time{},    // Not available
		},
		NetworkIPs: networkIPs,
		Hostname:   spec.Hostname,
		PID:        0, // Not available
	}, nil
}

func extractStatus(status containerd.ProcessStatus) workloadmeta.ContainerStatus {
	switch status {
	case containerd.Paused, containerd.Pausing:
		return workloadmeta.ContainerStatusPaused
	case containerd.Created:
		return workloadmeta.ContainerStatusCreated
	case containerd.Running:
		return workloadmeta.ContainerStatusRunning
	case containerd.Stopped:
		return workloadmeta.ContainerStatusStopped
	}

	return workloadmeta.ContainerStatusUnknown
}

// extractIP gets the IP of a container.
//
// The containerd client does not expose the IPs, that's why we use the helpers
// that we have in the "system" package to extract that information from
// "/proc".
//
// A current limitation is that if a container exposes multiple IPs, this
// function just returns one of them. That means that if a container is attached
// to multiple networks this might not work as expected.
func extractIP(container containerd.Container, containerdClient cutil.ContainerdItf) (string, error) {
	taskPids, err := containerdClient.TaskPids(container)
	if err != nil {
		return "", err
	}

	if len(taskPids) == 0 {
		return "", errors.New("no PIDs found")
	}

	IPs, err := system.ParseProcessIPs(
		config.Datadog.GetString("container_proc_root"),
		int(taskPids[0].Pid), // Any PID of the container should work
	)
	if err != nil {
		return "", err
	}

	// From all the IPs, just return the first one that's not localhost.
	for _, IP := range IPs {
		if IP != "127.0.0.1" {
			return IP, nil
		}
	}

	return "", errors.New("no IPs found")
}
