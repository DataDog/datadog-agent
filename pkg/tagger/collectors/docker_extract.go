// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build docker

package collectors

import (
	"fmt"
	"strings"

	"github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Allows to pass the dockerutil resolving method to
// dockerExtractImage while using a mock for tests
type resolveHook func(co types.ContainerJSON) (string, error)

// extractFromInspect extract tags for a container inspect JSON
func (c *DockerCollector) extractFromInspect(co types.ContainerJSON) ([]string, []string, []string, []string) {
	tags := utils.NewTagList()

	dockerExtractImage(tags, co, c.dockerUtil.ResolveImageNameFromContainer)
	dockerExtractLabels(tags, co.Config.Labels, c.labelsAsTags)
	dockerExtractEnvironmentVariables(tags, co.Config.Env, c.envAsTags)

	tags.AddHigh("container_name", strings.TrimPrefix(co.Name, "/"))
	tags.AddHigh("container_id", co.ID)

	low, orchestrator, high, standard := tags.Compute()
	return low, orchestrator, high, standard
}

func dockerExtractImage(tags *utils.TagList, co types.ContainerJSON, resolve resolveHook) {
	// Swarm / Compose will store the full image with tag and sha in co.Config.Image
	// while co.Image will miss the tag. Handle this case first before using the sha
	// and inspecting the image.
	if co.Config != nil && strings.Contains(co.Config.Image, "@sha256") {
		imageName, shortImage, imageTag, err := containers.SplitImageName(co.Config.Image)
		if err == nil && imageName != "" && imageTag != "" {
			tags.AddLow("docker_image", fmt.Sprintf("%s:%s", imageName, imageTag))
			tags.AddLow("image_name", imageName)
			tags.AddLow("short_image", shortImage)
			tags.AddLow("image_tag", imageTag)
			return
		}
	}

	// Resolve sha to image based on repotags/repodigests
	dockerImage, err := resolve(co)
	if err != nil {
		log.Debugf("Error resolving image %s: %s", co.Image, err)
		return
	}
	tags.AddLow("docker_image", dockerImage)

	imageName, shortImage, imageTag, err := containers.SplitImageName(dockerImage)
	if err != nil {
		// Fallback and try to parse co.Config.Image if exists
		if err == containers.ErrImageIsSha256 && co.Config != nil {
			var errFallback error
			imageName, shortImage, imageTag, errFallback = containers.SplitImageName(co.Config.Image)
			if errFallback != nil {
				log.Debugf("Cannot split %s: %s - fallback also failed: %s: %s ", dockerImage, err, co.Config.Image, errFallback)
				return
			}
		} else {
			log.Debugf("Cannot split %s: %s", dockerImage, err)
			return
		}
	}
	tags.AddLow("image_name", imageName)
	tags.AddLow("short_image", shortImage)
	tags.AddLow("image_tag", imageTag)
}

// dockerExtractLabels extracts tags from docker labels
// extracts env, version and service tags
// extracts labels as tags
// extracts hard-coded labels from:
// - Docker swarm
// - Rancher
// - Custom
func dockerExtractLabels(tags *utils.TagList, containerLabels map[string]string, labelsAsTags map[string]string) {
	for labelName, labelValue := range containerLabels {
		switch labelName {
		// Docker swarm
		case "com.docker.swarm.service.name":
			tags.AddLow("swarm_service", labelValue)
		case "com.docker.stack.namespace":
			tags.AddLow("swarm_namespace", labelValue)

		// Rancher 1.x
		case "io.rancher.container.name":
			tags.AddHigh("rancher_container", labelValue)
		case "io.rancher.stack.name":
			tags.AddLow("rancher_stack", labelValue)
		case "io.rancher.stack_service.name":
			tags.AddLow("rancher_service", labelValue)

		// Standard tags
		case dockerLabelEnv:
			tags.AddStandard(tagKeyEnv, labelValue)
		case dockerLabelVersion:
			tags.AddStandard(tagKeyVersion, labelValue)
		case dockerLabelService:
			tags.AddStandard(tagKeyService, labelValue)

		// Custom labels as tags
		case autodiscoveryLabelTagsKey:
			parseContainerADTagsLabels(tags, labelValue)
		default:
			if tagName, found := labelsAsTags[strings.ToLower(labelName)]; found {
				tags.AddAuto(tagName, labelValue)
			}
		}
	}
}

// dockerExtractEnvironmentVariables extracts tags from the container's environment variables
// extracts env, version and service tags
// extracts environment variables as tags
// extracts hard-coded environment variables from:
// - Mesos/DCOS tags (mesos, marathon, chronos)
// - Nomad
func dockerExtractEnvironmentVariables(tags *utils.TagList, containerEnvVariables []string, envAsTags map[string]string) {
	var envSplit []string
	var envName, envValue string

	for _, envEntry := range containerEnvVariables {
		envSplit = strings.SplitN(envEntry, "=", 2)
		if len(envSplit) != 2 {
			continue
		}
		envName = envSplit[0]
		envValue = envSplit[1]
		switch envName {
		// Mesos/DCOS tags (mesos, marathon, chronos)
		case "MARATHON_APP_ID":
			tags.AddLow("marathon_app", envValue)
		case "CHRONOS_JOB_NAME":
			tags.AddLow("chronos_job", envValue)
		case "CHRONOS_JOB_OWNER":
			tags.AddLow("chronos_job_owner", envValue)
		case "MESOS_TASK_ID":
			tags.AddOrchestrator("mesos_task", envValue)

		// Nomad
		case "NOMAD_TASK_NAME":
			tags.AddLow("nomad_task", envValue)
		case "NOMAD_JOB_NAME":
			tags.AddLow("nomad_job", envValue)
		case "NOMAD_GROUP_NAME":
			tags.AddLow("nomad_group", envValue)

		// Standard tags
		case envVarEnv:
			tags.AddStandard(tagKeyEnv, envValue)
		case envVarVersion:
			tags.AddStandard(tagKeyVersion, envValue)
		case envVarService:
			tags.AddStandard(tagKeyService, envValue)

		default:
			if tagName, found := envAsTags[strings.ToLower(envSplit[0])]; found {
				tags.AddAuto(tagName, envValue)
			}
		}
	}
}
