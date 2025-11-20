// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// containerTagCache encapsulates the logic for retrieving and caching container tags, to
// add as workload tags for GPU monitoring metrics.
type containerTagCache struct {
	cache  map[string][]string
	tagger tagger.Component
}

func newContainerTagCache(tagger tagger.Component) *containerTagCache {
	return &containerTagCache{
		cache:  make(map[string][]string),
		tagger: tagger,
	}
}

// getContainerTags retrieves the tags for a container from the cache or the tagger, and caches the result.
// It can return "nil, nil" if there was a previous error retrieving the tags for the container.
func (c *containerTagCache) getContainerTags(container *workloadmeta.Container) ([]string, error) {
	containerID := container.EntityID.ID
	if tags, exists := c.cache[containerID]; exists {
		return tags, nil
	}

	entityID := taggertypes.NewEntityID(taggertypes.ContainerID, containerID)

	// we use orchestrator cardinality here to ensure we get the pod_name tag
	// ref: https://docs.datadoghq.com/containers/kubernetes/tag/?tab=datadogoperator#out-of-the-box-tags
	cardinality := taggertypes.OrchestratorCardinality
	if container.Runtime == workloadmeta.ContainerRuntimeDocker {
		// For Docker, we use high cardinality to get the container_name and container_id tags
		// that uniquely identify the container.
		// ref: https://docs.datadoghq.com/containers/docker/tag/#out-of-the-box-tagging
		cardinality = taggertypes.HighCardinality
	}

	tags, err := c.tagger.Tag(entityID, cardinality)
	if err != nil {
		// Cache the error state to avoid repeated calls in case of errors
		c.cache[containerID] = nil
		return nil, err
	}

	c.cache[containerID] = tags
	return tags, nil
}
