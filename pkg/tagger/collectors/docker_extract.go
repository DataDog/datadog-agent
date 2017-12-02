// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package collectors

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	log "github.com/cihub/seelog"
	"github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

// extractFromInspect extract tags for a container inspect JSON
func (c *DockerCollector) extractFromInspect(co types.ContainerJSON) ([]string, []string, error) {
	tags := utils.NewTagList()

	c.recordImageTagsFromInspect(tags, co)
	c.recordLabelsFromInspect(tags, co.Config.Labels)
	c.recordEnvVariableFromInspect(tags, co.Config.Env)

	tags.AddHigh("container_name", strings.TrimPrefix(co.Name, "/"))
	tags.AddHigh("container_id", co.ID)

	low, high := tags.Compute()
	return low, high, nil
}

func (c *DockerCollector) recordImageTagsFromInspect(tags *utils.TagList, co types.ContainerJSON) {
	dockerImage, err := c.dockerUtil.ResolveImageName(co.Image)
	if err != nil {
		log.Debugf("error resolving image %s: %s", co.Image, err)
		return
	}
	imageName, _, imageTag, err := docker.SplitImageName(dockerImage)
	tags.AddLow("docker_image", dockerImage)
	if err != nil {
		log.Debugf("error splitting %s: %s", dockerImage, err)
		return
	}
	tags.AddLow("image_name", imageName)
	tags.AddLow("image_tag", imageTag)
}

func (c *DockerCollector) recordLabelsFromInspect(recordTags *utils.TagList, labels map[string]string) {
	for labelName, labelValue := range labels {
		if tagName, found := c.labelsAsTags[strings.ToLower(labelName)]; found {
			if tagName[0] == '+' {
				recordTags.AddHigh(tagName[1:], labelValue)
				continue
			}
			recordTags.AddLow(tagName, labelValue)
		}
	}
}

// recordEnvVariableFromInspect contain hard-coded environment variables from:
// - Mesos Marathon
func (c *DockerCollector) recordEnvVariableFromInspect(recordTags *utils.TagList, envVariables []string) {
	var envSplit []string

	for _, envEntry := range envVariables {
		envSplit = strings.SplitN(envEntry, "=", 2)
		if len(envSplit) != 2 {
			continue
		}
		switch envSplit[0] {
		// Mesos Marathon
		case "MARATHON_APP_ID":
			recordTags.AddLow("marathon_app", envSplit[1])
		case "CHRONOS_JOB_NAME":
			recordTags.AddLow("chronos_job", envSplit[1])
		case "CHRONOS_JOB_OWNER":
			recordTags.AddLow("chronos_job_owner", envSplit[1])
		case "MESOS_TASK_ID":
			recordTags.AddHigh("mesos_task", envSplit[1])

		default:
			if tagName, found := c.envAsTags[strings.ToLower(envSplit[0])]; found {
				if tagName[0] == '+' {
					recordTags.AddHigh(tagName[1:], envSplit[1])
					continue
				}
				recordTags.AddLow(tagName, envSplit[1])
			}
		}
	}
}
