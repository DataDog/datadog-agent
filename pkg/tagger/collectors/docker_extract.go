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

type extract struct {
	tags            *utils.TagList
	dockerCollector *DockerCollector
}

func newExtractor(c *DockerCollector) *extract {
	return &extract{
		tags:            utils.NewTagList(),
		dockerCollector: c,
	}
}

// extractFromInspect extract tags for a container inspect JSON
func (c *DockerCollector) extractFromInspect(co types.ContainerJSON) ([]string, []string, error) {
	ex := newExtractor(c)

	ex.extractImage(co)
	ex.extractLabels(co.Config.Labels)
	ex.extractEnvironmentVariables(co.Config.Env)

	ex.tags.AddHigh("container_name", strings.TrimPrefix(co.Name, "/"))
	ex.tags.AddHigh("container_id", co.ID)

	low, high := ex.tags.Compute()
	return low, high, nil
}

func (e *extract) extractImage(co types.ContainerJSON) {
	dockerImage, err := e.dockerCollector.dockerUtil.ResolveImageName(co.Image)
	if err != nil {
		log.Debugf("error resolving image %s: %s", co.Image, err)
		return
	}
	imageName, _, imageTag, err := docker.SplitImageName(dockerImage)
	e.tags.AddLow("docker_image", dockerImage)
	if err != nil {
		log.Debugf("error splitting %s: %s", dockerImage, err)
		return
	}
	e.tags.AddLow("image_name", imageName)
	e.tags.AddLow("image_tag", imageTag)
}

func (e *extract) extractLabels(labels map[string]string) {
	for labelName, labelValue := range labels {
		if tagName, found := e.dockerCollector.labelsAsTags[strings.ToLower(labelName)]; found {
			if tagName[0] == '+' {
				e.tags.AddHigh(tagName[1:], labelValue)
				continue
			}
			e.tags.AddLow(tagName, labelValue)
		}
	}
}

// extractEnvironmentVariables contain hard-coded environment variables from:
// - Mesos Marathon
func (e *extract) extractEnvironmentVariables(envVariables []string) {
	var envSplit []string

	for _, envEntry := range envVariables {
		envSplit = strings.SplitN(envEntry, "=", 2)
		if len(envSplit) != 2 {
			continue
		}
		switch envSplit[0] {
		// Mesos Marathon
		case "MARATHON_APP_ID":
			e.tags.AddLow("marathon_app", envSplit[1])
		case "CHRONOS_JOB_NAME":
			e.tags.AddLow("chronos_job", envSplit[1])
		case "CHRONOS_JOB_OWNER":
			e.tags.AddLow("chronos_job_owner", envSplit[1])
		case "MESOS_TASK_ID":
			e.tags.AddHigh("mesos_task", envSplit[1])

		default:
			if tagName, found := e.dockerCollector.envAsTags[strings.ToLower(envSplit[0])]; found {
				if tagName[0] == '+' {
					e.tags.AddHigh(tagName[1:], envSplit[1])
					continue
				}
				e.tags.AddLow(tagName, envSplit[1])
			}
		}
	}
}
