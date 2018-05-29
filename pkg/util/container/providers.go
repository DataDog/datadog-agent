// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.
// +build docker

package container

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/ecs"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// IsAvailable returns true if there's at least one container provider, false otherwise
func IsAvailable() bool {
	var listeners []config.Listeners
	if err := config.Datadog.UnmarshalKey("listeners", &listeners); err != nil {
		log.Errorf("unable to parse get listeners from the datadog config - %s", err)
	}

	if len(listeners) > 0 {
		return true
	}
	return false
}

// GetContainers is the unique method that returns all containers on the host (or in the task)
// TODO: create a container interface that docker and ecs can implement
// and that other agents can consume so that we don't have to
// convert all containers to the format.
// TODO: move to a catalog and registration pattern
func GetContainers() ([]*docker.Container, error) {
	var listeners []config.Listeners
	var err error
	if err = config.Datadog.UnmarshalKey("listeners", &listeners); err != nil {
		log.Errorf("unable to parse listeners from the datadog config - %s", err)
		return nil, err
	}

	containers := make([]*docker.Container, 0)
	ctrListConfig := docker.ContainerListConfig{
		IncludeExited: false,
		FlagExcluded:  false,
	}

	for _, l := range listeners {
		switch l.Name {
		case "docker":
			du, err := docker.GetDockerUtil()
			if err != nil {
				log.Errorf("unable to connect to docker, passing this provider - %s", err)
				continue
			}
			ctrs, err := du.Containers(&ctrListConfig)
			if err != nil {
				log.Errorf("failed to get container list from docker - %s", err)
			}
			containers = append(containers, ctrs...)
		case "ecs":
			ctrs, err := ecs.GetContainers()
			if err != nil {
				log.Errorf("failed to get container list from ecs - %s", err)
			}
			containers = append(containers, ctrs...)
		default:
			log.Warnf("listener %s is not a known container provider, skipping it", l.Name)
		}
	}
	return containers, err
}
