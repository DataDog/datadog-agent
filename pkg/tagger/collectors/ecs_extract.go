// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package collectors

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	ecsutil "github.com/DataDog/datadog-agent/pkg/util/ecs"
)

func (c *ECSCollector) parseTasks(tasks_list ecsutil.TasksV1Response, targetDockerID string) ([]*TagInfo, error) {
	var output []*TagInfo
	now := time.Now()
	for _, task := range tasks_list.Tasks {
		// We only want to collect tasks without a STOPPED status.
		if task.KnownStatus == "STOPPED" {
			continue
		}
		for _, container := range task.Containers {
			// Only collect new containers + the targeted container, to avoid empty tags on race conditions
			if c.expire.Update(container.DockerID, now) || container.DockerID == targetDockerID {
				tags := utils.NewTagList()
				tags.AddLow("task_version", task.Version)
				tags.AddLow("task_name", task.Family)
				tags.AddLow("task_family", task.Family)
				tags.AddLow("ecs_container_name", container.Name)

				if c.clusterName != "" {
					tags.AddLow("cluster_name", c.clusterName)
				}

				tags.AddOrchestrator("task_arn", task.Arn)

				low, orch, high := tags.Compute()

				info := &TagInfo{
					Source:               ecsCollectorName,
					Entity:               containers.BuildTaggerEntityName(container.DockerID),
					HighCardTags:         high,
					OrchestratorCardTags: orch,
					LowCardTags:          low,
				}
				output = append(output, info)
			}
		}
	}
	return output, nil
}
