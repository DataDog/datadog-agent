// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package providers

import (
	"sync"

	log "github.com/cihub/seelog"

	autodiscovery "github.com/DataDog/datadog-agent/pkg/autodiscovery/config"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

const (
	dockerADLabelPrefix = "com.datadoghq.ad."
)

// DockerConfigProvider implements the ConfigProvider interface for the docker labels.
type DockerConfigProvider struct {
	sync.RWMutex
	dockerUtil *docker.DockerUtil
	upToDate   bool
	streaming  bool
	health     *health.Handle
}

// NewDockerConfigProvider returns a new ConfigProvider connected to docker.
// Connectivity is not checked at this stage to allow for retries, Collect will do it.
func NewDockerConfigProvider(config config.ConfigurationProviders) (ConfigProvider, error) {
	return &DockerConfigProvider{}, nil
}

// String returns a string representation of the DockerConfigProvider
func (d *DockerConfigProvider) String() string {
	return "Docker container labels"
}

// Collect retrieves all running containers and extract AD templates from their labels.
func (d *DockerConfigProvider) Collect() ([]autodiscovery.Config, error) {
	var err error
	if d.dockerUtil == nil {
		d.dockerUtil, err = docker.GetDockerUtil()
		if err != nil {
			return []autodiscovery.Config{}, err
		}
		go d.listen()
	}

	containers, err := d.dockerUtil.AllContainerLabels()
	if err != nil {
		return []autodiscovery.Config{}, err
	}

	d.Lock()
	d.upToDate = true
	d.Unlock()

	return parseDockerLabels(containers)
}

// We listen to docker events and invalidate our cache when we receive a start/die event
func (d *DockerConfigProvider) listen() {
	d.Lock()
	d.streaming = true
	d.health = health.Register("ad-dockerprovider")
	d.Unlock()

CONNECT:
	for {
		eventChan, errChan, err := d.dockerUtil.SubscribeToContainerEvents(d.String())
		if err != nil {
			log.Warnf("error subscribing to docker events: %s", err)
			break CONNECT // We disable streaming and revert to always-pull behaviour
		}

		for {
			select {
			case <-d.health.C:
			case ev := <-eventChan:
				// As our input is the docker `client.ContainerList`, which lists running containers,
				// only these two event types will change what containers appear.
				// Container labels cannot change once they are created, so we don't need to react on
				// other lifecycle events.
				if ev.Action == "start" || ev.Action == "die" {
					d.Lock()
					d.upToDate = false
					d.Unlock()
				}
			case err := <-errChan:
				log.Warnf("error getting docker events: %s", err)
				d.Lock()
				d.upToDate = false
				d.Unlock()
				continue CONNECT // Re-connect to dockerutils
			}
		}
	}

	d.Lock()
	d.streaming = false
	d.health.Deregister()
	d.Unlock()
}

// IsUpToDate checks whether we have new containers to parse, based on events received by the listen goroutine.
// If listening fails, we fallback to Collecting everytime.
func (d *DockerConfigProvider) IsUpToDate() (bool, error) {
	d.RLock()
	defer d.RUnlock()
	return (d.streaming && d.upToDate), nil
}

func parseDockerLabels(containers map[string]map[string]string) ([]autodiscovery.Config, error) {
	var configs []autodiscovery.Config
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
