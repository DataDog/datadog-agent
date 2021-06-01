// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

package providers

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	v2 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2"
)

const (
	ecsADLabelPrefix = "com.datadoghq.ad."
)

// ECSConfigProvider implements the ConfigProvider interface.
// It collects configuration templates from the ECS metadata API.
type ECSConfigProvider struct {
	client *v2.Client
}

// NewECSConfigProvider returns a new ECSConfigProvider.
// It configures an http Client with a 500 ms timeout.
func NewECSConfigProvider(config config.ConfigurationProviders) (ConfigProvider, error) {
	client, err := ecsmeta.V2()
	if err != nil {
		log.Debugf("error while initializing ECS metadata V2 client: %s", err)
		return nil, err
	}

	return &ECSConfigProvider{
		client: client,
	}, nil
}

// String returns a string representation of the ECSConfigProvider
func (p *ECSConfigProvider) String() string {
	return names.ECS
}

// IsUpToDate updates the list of AD templates versions in the Agent's cache and checks the list is up to date compared to ECS' data.
func (p *ECSConfigProvider) IsUpToDate() (bool, error) {
	return false, nil
}

// Collect finds all running containers in the agent's task, reads their labels
// and extract configuration templates from them for auto discovery.
func (p *ECSConfigProvider) Collect() ([]integration.Config, error) {
	meta, err := p.client.GetTask()
	if err != nil {
		return nil, err
	}

	return parseECSContainers(meta.Containers)
}

// parseECSContainers loops through ecs containers found in the ecs metadata response
// and extracts configuration templates out of their labels.
func parseECSContainers(containers []v2.Container) ([]integration.Config, error) {
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

// GetConfigErrors is not implemented for the ECSConfigProvider
func (p *ECSConfigProvider) GetConfigErrors() map[string]ErrorMsgSet {
	return make(map[string]ErrorMsgSet)
}
