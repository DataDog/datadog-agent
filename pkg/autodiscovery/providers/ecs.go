// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package providers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/ecs"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	ecsADLabelPrefix        = "com.datadoghq.ad."
	metadataURL      string = "http://169.254.170.2/v2/metadata"
)

// ECSConfigProvider implements the ConfigProvider interface.
// It collects configuration templates from the ECS metadata API.
type ECSConfigProvider struct {
	client http.Client
}

// NewECSConfigProvider returns a new ECSConfigProvider.
// It configures an http Client with a 500 ms timeout.
func NewECSConfigProvider(config config.ConfigurationProviders) (ConfigProvider, error) {
	c := http.Client{
		Timeout: 500 * time.Millisecond,
	}
	return &ECSConfigProvider{
		client: c,
	}, nil
}

// String returns a string representation of the ECSConfigProvider
func (p *ECSConfigProvider) String() string {
	return ECS
}

// IsUpToDate updates the list of AD templates versions in the Agent's cache and checks the list is up to date compared to ECS' data.
func (p *ECSConfigProvider) IsUpToDate() (bool, error) {
	return false, nil
}

// Collect finds all running containers in the agent's task, reads their labels
// and extract configuration templates from them for auto discovery.
func (p *ECSConfigProvider) Collect() ([]integration.Config, error) {
	meta, err := p.getTaskMetadata()
	if err != nil {
		return nil, err
	}
	return parseECSContainers(meta.Containers)
}

// getTaskMetadata queries the ECS metadata API and unmarshals the resulting json
// into a TaskMetadata object.
func (p *ECSConfigProvider) getTaskMetadata() (ecs.TaskMetadata, error) {
	var meta ecs.TaskMetadata
	resp, err := p.client.Get(metadataURL)
	if err != nil {
		log.Errorf("unable to get task metadata - %s", err)
		return meta, err
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&meta)
	if err != nil {
		log.Errorf("unable to decode task metadata response - %s", err)
	}
	return meta, err
}

// parseECSContainers loops through ecs containers found in the ecs metadata response
// and extracts configuration templates out of their labels.
func parseECSContainers(containers []ecs.Container) ([]integration.Config, error) {
	var templates []integration.Config
	for _, c := range containers {
		dockerEntityName := docker.ContainerIDToEntityName(c.DockerID)
		configs, errors := extractTemplatesFromMap(dockerEntityName, c.Labels, ecsADLabelPrefix)

		for _, err := range errors {
			log.Errorf("unable to extract templates for container %s - %s", c.DockerID, err)
		}

		for idx := range configs {
			configs[idx].Source = "ecs:" + dockerEntityName
		}

		templates = append(templates, configs...)
	}
	return templates, nil
}

func init() {
	RegisterProvider("ecs", NewECSConfigProvider)
}
