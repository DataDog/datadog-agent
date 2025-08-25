// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

// Package docker contains utility functions for creating filterable objects.
package docker

import (
	"github.com/docker/docker/api/types/container"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	typedef "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def/proto"
)

// CreateContainer creates a filterable container from the docker container summary
func CreateContainer(rawContainer container.Summary, resolvedImageName string, owner workloadfilter.Filterable) *workloadfilter.Container {
	// We do reports some metrics about excluded containers, but the tagger won't have tags
	// We always use rawContainer.Names[0] to match historic behavior
	var containerName string
	if len(rawContainer.Names) > 0 {
		containerName = rawContainer.Names[0]
	}

	c := &typedef.FilterContainer{
		Id:    rawContainer.ID,
		Name:  containerName,
		Image: resolvedImageName,
	}

	if owner != nil {
		switch o := owner.(type) {
		case *workloadfilter.Pod:
			if o != nil && o.FilterPod != nil {
				c.Owner = &typedef.FilterContainer_Pod{
					Pod: o.FilterPod,
				}
			}
		}
	}

	return &workloadfilter.Container{
		FilterContainer: c,
		Owner:           owner,
	}
}
