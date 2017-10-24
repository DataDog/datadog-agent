// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package collectors

import (
	"fmt"
	ecsutil "github.com/DataDog/datadog-agent/pkg/util/ecs"
)

const (
	ecsCollectorName = "ecs"
)

// ECSCollector listen to the ECS agent to get ECS metadata.
// And feed a stream of TagInfo.

type ECSCollector struct {
	infoOut chan<- []*TagInfo
}

// Detect tries to connect to the ecs agent
func (c *ECSCollector) Detect(out chan<- []*TagInfo) (CollectionMode, error) {
	if ecsutil.IsInstance() {
		c.infoOut = out
		return FetchOnlyCollection, nil
	} else {
		return NoCollection, fmt.Errorf("Failed to connect to ecs, ECS tagging will not work")
	}

}

// Fetch fetches ECS tags
func (c *ECSCollector) Fetch(container string) ([]string, []string, error) {

	tasks_list, err := ecsutil.GetTasks()
	if err != nil {
		return []string{}, []string{}, err
	}
	updates, err := c.parseTasks(tasks_list)
	if err != nil {
		return []string{}, []string{}, err
	}
	c.infoOut <- updates

	for _, info := range updates {
		if info.Entity == container {
			return info.LowCardTags, info.HighCardTags, nil
		}
	}
	// container not found in updates
	return []string{}, []string{}, fmt.Errorf("entity %s not found in tasklist", container)
}

func ecsFactory() Collector {
	return &ECSCollector{}
}

func init() {
	registerCollector(ecsCollectorName, ecsFactory)
}
