// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package collectors

import (
	"fmt"
	"strings"

	log "github.com/cihub/seelog"
	"github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

// extractFromInspect extract tags for a container inspect JSON
func (c *DockerCollector) extractFromInspect(co types.ContainerJSON) ([]string, []string, error) {
	var low, high []string

	docker_image, err := docker.ResolveImageName(co.Image)
	if err != nil {
		log.Debugf("error resolving image %s: %s", co.Image, err)

	} else {
		image_name, image_tag, err := docker.SplitImageName(docker_image)

		low = append(low, fmt.Sprintf("docker_image:%s", docker_image))

		if err != nil {
			log.Debugf("error spliting %s: %s", docker_image, err)
		} else {
			low = append(low, fmt.Sprintf("image_name:%s", image_name))
			low = append(low, fmt.Sprintf("image_tag:%s", image_tag))
		}
	}

	for _, label := range c.labelsAsTags {
		if value, found := co.Config.Labels[label]; found {
			low = append(low, fmt.Sprintf("%s:%s", label, value))
		}
	}

	if len(c.envAsTags) > 0 {
		for _, envvar := range co.Config.Env {
			parts := strings.SplitN(envvar, "=", 2)
			if len(parts) != 2 {
				continue
			}
			for _, envAsTagConfigEntry := range c.envAsTags {
				if parts[0] == envAsTagConfigEntry {
					low = append(low, fmt.Sprintf("%s:%s", parts[0], parts[1]))
				}
			}
		}
	}

	high = append(high, fmt.Sprintf("container_name:%s", co.Name))
	high = append(high, fmt.Sprintf("container_id:%s", co.ID))

	return low, high, nil
}
