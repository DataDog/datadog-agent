// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package collectors

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/ecs"
)

// ignore these container labels as we already have them in task metadata
var isBlacklisted = map[string]bool{
	"com.amazonaws.ecs.cluster":                 true,
	"com.amazonaws.ecs.container-name":          true,
	"com.amazonaws.ecs.task-arn":                true,
	"com.amazonaws.ecs.task-definition-family":  true,
	"com.amazonaws.ecs.task-definition-version": true,
}

// pullMetadata parses the the task metadata, and its container list, and returns a list of TagInfo for the new ones.
// It also updates the lastSeen cache of the ECSFargateCollector and return the list of dead containers to be expired.
func (c *ECSFargateCollector) pullMetadata(meta ecs.TaskMetadata) ([]*TagInfo, []string, error) {
	var output []*TagInfo
	seen := make(map[string]interface{}, len(meta.Containers))

	if meta.KnownStatus != "RUNNING" {
		return output, nil, fmt.Errorf("Task %s is in %s status, skipping", meta.Family, meta.KnownStatus)
	}

	for _, ctr := range meta.Containers {
		seen[ctr.DockerID] = nil
		if _, found := c.lastSeen[ctr.DockerID]; !found {
			tags := utils.NewTagList()

			// cluster
			tags.AddLow("cluster_name", meta.ClusterName)

			// task
			tags.AddLow("task_family", meta.Family)
			tags.AddLow("task_version", meta.Version)

			// container
			tags.AddLow("ecs_container_name", ctr.Name)
			tags.AddHigh("container_name", ctr.DockerName)

			// container image
			image := ctr.Image
			tags.AddLow("docker_image", image)
			imageSplit := strings.Split(image, ":")
			imageName := strings.Join(imageSplit[:len(imageSplit)-1], ":")
			tags.AddLow("image_name", imageName)
			if len(imageSplit) > 1 {
				imageTag := imageSplit[len(imageSplit)-1]
				tags.AddLow("image_tag", imageTag)
			}

			// container labels
			for k, v := range ctr.Labels {
				if isBlacklisted[k] {
					tags.AddHigh(k, v)
				}
			}

			low, high := tags.Compute()
			info := &TagInfo{
				Source:       ecsFargateCollectorName,
				Entity:       docker.ContainerIDToEntityName(string(ctr.DockerID)),
				HighCardTags: high,
				LowCardTags:  low,
			}
			output = append(output, info)
		}
	}

	// compute containers that disappeared
	deadContainers := []string{}
	for ctr := range c.lastSeen {
		if _, found := seen[ctr]; !found {
			deadContainers = append(deadContainers, ctr)
		}
	}
	c.lastSeen = seen
	return output, deadContainers, nil
}

// fetchMetadata looks for a given container in a TaskMetadata object and returns its tags if found.
func (c *ECSFargateCollector) fetchMetadata(meta ecs.TaskMetadata, container string) ([]string, []string, error) {
	for _, ctr := range meta.Containers {
		entity := docker.ContainerIDToEntityName(string(ctr.DockerID))
		if entity != container {
			continue
		}
		tags := utils.NewTagList()

		// cluster
		tags.AddLow("cluster_name", meta.ClusterName)

		// task
		tags.AddLow("task_family", meta.Family)
		tags.AddLow("task_version", meta.Version)

		// container
		tags.AddLow("ecs_container_name", ctr.Name)
		tags.AddHigh("container_name", ctr.DockerName)

		// container image
		image := ctr.Image
		tags.AddLow("docker_image", image)
		imageSplit := strings.Split(image, ":")
		imageName := strings.Join(imageSplit[:len(imageSplit)-1], ":")
		tags.AddLow("image_name", imageName)
		if len(imageSplit) > 1 {
			imageTag := imageSplit[len(imageSplit)-1]
			tags.AddLow("image_tag", imageTag)
		}

		// container labels
		for k, v := range ctr.Labels {
			if isBlacklisted[k] {
				tags.AddHigh(k, v)
			}
		}

		low, high := tags.Compute()
		info := &TagInfo{
			Source:       ecsFargateCollectorName,
			Entity:       docker.ContainerIDToEntityName(string(ctr.DockerID)),
			HighCardTags: high,
			LowCardTags:  low,
		}
		return info.LowCardTags, info.HighCardTags, nil
	}
	return nil, nil, errors.NewNotFound(fmt.Sprintf("%s/%s", meta.TaskARN, container))
}
