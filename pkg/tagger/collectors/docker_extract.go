// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package collectors

import (
	"fmt"

	log "github.com/cihub/seelog"
	"github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

// ExtractFromInspect extract tags for a container inspect JSON
func (c *DockerCollector) ExtractFromInspect(co types.ContainerJSON) ([]string, []string, error) {
	var low, high []string

	docker_image, _ := docker.ResolveImageName(co.Image)
	image_name, image_tag, err := docker.SplitImageName(docker_image)

	low = append(low, fmt.Sprintf("docker_image:%s", docker_image))
	low = append(low, fmt.Sprintf("image_name:%s", image_name))
	low = append(low, fmt.Sprintf("image_tag:%s", image_tag))

	if err != nil {
		log.Debugf("error spliting %s: %s", docker_image, err)
	}

	for _, label := range c.labelsAsTags {
		if value, found := co.Config.Labels[label]; found {
			low = append(low, fmt.Sprintf("%s:%s", label, value))
		}
	}

	// TODO: to be completed once docker utils are merged
	low = append(low, "low:test")
	high = append(high, fmt.Sprintf("container_name:%s", co.ID))
	return low, high, nil
}
