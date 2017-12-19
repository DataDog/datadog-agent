// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package providers

import (
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

const (
	dockerADLabelPrefix = "com.datadoghq.ad."
)

// DockerConfigProvider implements the ConfigProvider interface for the docker labels.
type DockerConfigProvider struct {
	dockerUtil *docker.DockerUtil
}

// NewDockerConfigProvider returns a new ConfigProvider connected to docker.
// Connectivity is not checked at this stage to allow for retries, Collect will do it.
func NewDockerConfigProvider(config config.ConfigurationProviders) (ConfigProvider, error) {
	return &DockerConfigProvider{}, nil
}

func (d *DockerConfigProvider) String() string {
	return "Docker container labels"
}

// Collect retrieves all running containers and extract AD templates from their labels.
// TODO: suscribe to docker events and only invalidate cache if we get a `start` event since last Collect.
func (d *DockerConfigProvider) Collect() ([]check.Config, error) {
	var err error
	if d.dockerUtil == nil {
		d.dockerUtil, err = docker.GetDockerUtil()
		if err != nil {
			return []check.Config{}, err
		}
	}

	containers, err := d.dockerUtil.AllContainerLabels()
	if err != nil {
		return []check.Config{}, err
	}

	return parseDockerLabels(containers)
}

// IsUpToDate updates the list of AD templates versions in the Agent's cache and checks the list is up to date compared to Docker's data.
func (d *DockerConfigProvider) IsUpToDate() (bool, error) {
	return false, nil
}

func parseDockerLabels(containers map[string]map[string]string) ([]check.Config, error) {
	var configs []check.Config
	for cID, labels := range containers {
		c, err := extractTemplatesFromMap(docker.ContainerIDToEntityName(cID), labels, dockerADLabelPrefix)
		switch {
		case err != nil:
			log.Errorf("Can't parse template for container %s: %s", cID, err)
			continue
		case len(c) == 0:
			continue
		default:
			configs = append(configs, c...)

		}
	}
	return configs, nil
}

func init() {
	RegisterProvider("docker", NewDockerConfigProvider)
}
