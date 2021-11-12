// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd
// +build containerd

package containerd

import (
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"

	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// buildWorkloadMetaContainer generates a workloadmeta.Container from a containerd.Container
func buildWorkloadMetaContainer(container containerd.Container, containerdClient cutil.ContainerdItf) (workloadmeta.Container, error) {
	labels, err := containerdClient.Labels(container)
	if err != nil {
		return workloadmeta.Container{}, err
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

	containerdImage, err := containerdClient.Image(container)
	if err != nil {
		return workloadmeta.Container{}, err
	}

	imageName := containerdImage.Name()
	image, err := workloadmeta.NewContainerImage(imageName)
	if err != nil {
		log.Debugf("cannot split image name %q: %s", imageName, err)
	}

	status, err := containerdClient.Status(container)
	if err != nil {
		if !errdefs.IsNotFound(err) {
			return workloadmeta.Container{}, err
		}

		// The container exists, but there isn't a task associated to it. That
		// means that the container is not running, which is all we need to know
		// in this function (we can set any status != containerd.Running).
		status = ""
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
			Labels: labels,
		},
		Image:   image,
		EnvVars: envs,
		Ports:   nil, // Not available
		Runtime: workloadmeta.ContainerRuntimeContainerd,
		State: workloadmeta.ContainerState{
			Running:    status == containerd.Running,
			StartedAt:  info.CreatedAt,
			FinishedAt: time.Time{}, // Not available
		},
		NetworkIPs: make(map[string]string), // Not available
		Hostname:   spec.Hostname,
		PID:        0, // Not available
	}, nil
}
