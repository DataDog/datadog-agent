// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package collectors

import (
	"strings"

	log "github.com/cihub/seelog"
	"github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

// extractFromInspect extract tags for a container inspect JSON
func (c *DockerCollector) extractFromInspect(co types.ContainerJSON) ([]string, []string, error) {
	tags := utils.NewTagList()

	//TODO: remove when Inspect returns resolved image names
	dockerImage, err := c.dockerUtil.ResolveImageName(co.Image)
	if err != nil {
		log.Debugf("error resolving image %s: %s", co.Image, err)
	} else {
		dockerExtractImage(tags, dockerImage)
	}
	dockerExtractLabels(tags, co.Config.Labels, c.labelsAsTags)
	dockerExtractEnvironmentVariables(tags, co.Config.Env, c.envAsTags)

	tags.AddHigh("container_name", strings.TrimPrefix(co.Name, "/"))
	tags.AddHigh("container_id", co.ID)

	low, high := tags.Compute()
	return low, high, nil
}

func dockerExtractImage(tags *utils.TagList, dockerImage string) {
	tags.AddLow("docker_image", dockerImage)
	imageName, _, imageTag, err := docker.SplitImageName(dockerImage)
	if err != nil {
		log.Debugf("error splitting %s: %s", dockerImage, err)
		return
	}
	tags.AddLow("image_name", imageName)
	tags.AddLow("image_tag", imageTag)
}

// dockerExtractLabels contain hard-coded labels from:
// - Docker swarm
func dockerExtractLabels(tags *utils.TagList, containerLabels map[string]string, labelsAsTags map[string]string) {

	for labelName, labelValue := range containerLabels {
		switch labelName {
		// Docker swarm
		case "com.docker.swarm.service.name":
			tags.AddLow("swarm_service", labelValue)

		default:
			if tagName, found := labelsAsTags[strings.ToLower(labelName)]; found {
				tags.AddAuto(tagName, labelValue)
			}
		}
	}
}

// dockerExtractEnvironmentVariables contain hard-coded environment variables from:
// - Mesos/DCOS tags (mesos, marathon, chronos)
func dockerExtractEnvironmentVariables(tags *utils.TagList, containerEnvVariables []string, envAsTags map[string]string) {
	var envSplit []string
	var envName, envValue string

	for _, envEntry := range containerEnvVariables {
		envSplit = strings.SplitN(envEntry, "=", 2)
		envName = envSplit[0]
		envValue = envSplit[1]
		if len(envSplit) != 2 {
			continue
		}
		switch envName {
		// Mesos/DCOS tags (mesos, marathon, chronos)
		case "MARATHON_APP_ID":
			tags.AddLow("marathon_app", envValue)
		case "CHRONOS_JOB_NAME":
			tags.AddLow("chronos_job", envValue)
		case "CHRONOS_JOB_OWNER":
			tags.AddLow("chronos_job_owner", envValue)
		case "MESOS_TASK_ID":
			tags.AddHigh("mesos_task", envValue)

		default:
			if tagName, found := envAsTags[strings.ToLower(envSplit[0])]; found {
				tags.AddAuto(tagName, envValue)
			}
		}
	}
}
