// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-2020 Datadog, Inc.

// +build docker

package collectors

import (
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	v2 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// parseMetadata parses the task metadata and its container list, and returns a list of TagInfo for the new ones.
// It also updates the lastSeen cache of the ECSFargateCollector and return the list of dead containers to be expired.
func (c *ECSFargateCollector) parseMetadata(meta *v2.Task, parseAll bool) ([]*TagInfo, error) {
	var output []*TagInfo
	now := time.Now()

	if meta.KnownStatus != "RUNNING" {
		return output, fmt.Errorf("Task %s is in %s status, skipping", meta.Family, meta.KnownStatus)
	}

	c.doOnceOrchScope.Do(func() {
		tags := utils.NewTagList()
		tags.AddOrchestrator("task_arn", meta.TaskARN)
		low, orch, high := tags.Compute()
		info := &TagInfo{
			Source:               ecsFargateCollectorName,
			Entity:               OrchestratorScopeEntityID,
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          low,
		}
		output = append(output, info)
	})

	for _, ctr := range meta.Containers {
		if c.expire.Update(ctr.DockerID, now) || parseAll {
			tags := utils.NewTagList()

			// cluster
			clusterName := parseECSClusterName(meta.ClusterName)
			if !config.Datadog.GetBool("disable_cluster_name_tag_key") {
				tags.AddLow("cluster_name", clusterName)
			}
			tags.AddLow("ecs_cluster_name", clusterName)

			// aws region from cluster arn
			region := parseFargateRegion(meta.ClusterName)
			if region != "" {
				tags.AddLow("region", region)
			}

			// the AvailabilityZone metadata is only available for
			// Fargate tasks using platform version 1.4 or later
			availabilityZone := meta.AvailabilityZone
			if availabilityZone != "" {
				tags.AddLow("availability_zone", availabilityZone)
			}

			// task
			tags.AddLow("task_family", meta.Family)
			tags.AddLow("task_version", meta.Version)
			tags.AddOrchestrator("task_arn", meta.TaskARN)

			// container
			tags.AddLow("ecs_container_name", ctr.Name)
			tags.AddHigh("container_id", ctr.DockerID)
			tags.AddHigh("container_name", ctr.DockerName)

			// container image
			tags.AddLow("docker_image", ctr.Image)
			imageName, shortImage, imageTag, err := containers.SplitImageName(ctr.Image)
			if err != nil {
				log.Debugf("Cannot split %s: %s", ctr.Image, err)
			} else {
				tags.AddLow("image_name", imageName)
				tags.AddLow("short_image", shortImage)
				if imageTag == "" {
					imageTag = "latest"
				}
				tags.AddLow("image_tag", imageTag)
			}

			for labelName, labelValue := range ctr.Labels {
				switch labelName {
				case dockerLabelEnv:
					tags.AddLow(tagKeyEnv, labelValue)
				case dockerLabelVersion:
					tags.AddLow(tagKeyVersion, labelValue)
				case dockerLabelService:
					tags.AddLow(tagKeyService, labelValue)
				}

				if tagName, found := c.labelsAsTags[strings.ToLower(labelName)]; found {
					tags.AddAuto(tagName, labelValue)
				}
			}

			low, orch, high := tags.Compute()
			info := &TagInfo{
				Source:               ecsFargateCollectorName,
				Entity:               containers.BuildTaggerEntityName(ctr.DockerID),
				HighCardTags:         high,
				OrchestratorCardTags: orch,
				LowCardTags:          low,
			}
			output = append(output, info)
		}
	}

	return output, nil
}

// parseECSClusterName allows to handle user-friendly values and arn values
func parseECSClusterName(value string) string {
	if strings.Contains(value, "/") {
		parts := strings.Split(value, "/")
		return parts[len(parts)-1]
	}

	return value
}

func parseFargateRegion(arn string) string {
	arnParts := strings.Split(arn, ":")
	if len(arnParts) < 4 {
		return ""
	}
	if arnParts[0] != "arn" || arnParts[1] != "aws" {
		return ""
	}
	region := arnParts[3]

	// Sanity check
	if strings.Count(region, "-") < 2 {
		return ""
	}

	return region
}
