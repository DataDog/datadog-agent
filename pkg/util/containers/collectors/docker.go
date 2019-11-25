// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package collectors

import (
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

const (
	dockerCollectorName = "docker"
)

// DockerCollector lists containers from the docker socket and populates
// performance metric from the linux cgroups
type DockerCollector struct {
	dockerUtil *docker.DockerUtil
	listConfig *docker.ContainerListConfig
}

// Detect tries to connect to the docker socket and returns success
func (c *DockerCollector) Detect() error {
	du, err := docker.GetDockerUtil()
	if err != nil {
		return err
	}

	c.dockerUtil = du
	c.listConfig = &docker.ContainerListConfig{
		IncludeExited: false,
		FlagExcluded:  false,
	}
	return nil
}

// List gets all running containers
func (c *DockerCollector) List() ([]*containers.Container, error) {
	return c.dockerUtil.ListContainers(c.listConfig)
}

// UpdateMetrics updates metrics on an existing list of containers
func (c *DockerCollector) UpdateMetrics(cList []*containers.Container) error {
	return c.dockerUtil.UpdateContainerMetrics(cList)
}

func dockerFactory() Collector {
	return &DockerCollector{}
}

func init() {
	registerCollector(dockerCollectorName, dockerFactory, NodeRuntime)
}
