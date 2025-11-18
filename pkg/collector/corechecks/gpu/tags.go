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

func (c *containerTagCache) getContainerTags(container *workloadmeta.Container) ([]string, error) {
	containerID := container.EntityID.ID
	if tags, exists := c.cache[containerID]; exists {
		return tags, nil
	}

	entityID := taggertypes.NewEntityID(taggertypes.ContainerID, containerID)

	cardinality := taggertypes.OrchestratorCardinality
	if container.Runtime == workloadmeta.ContainerRuntimeDocker {
		cardinality = taggertypes.HighCardinality
	}

	tags, err := c.tagger.Tag(entityID, cardinality)
	if err != nil {
		// Cache the error state to avoid repeated calls
		c.cache[containerID] = nil
		return nil, err
	}

	c.cache[containerID] = tags
	return tags, nil
}
