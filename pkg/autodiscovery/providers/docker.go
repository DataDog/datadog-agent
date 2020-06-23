// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build docker

package providers

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

const (
	dockerADLabelPrefix = "com.datadoghq.ad."
	delayDuration       = 5 * time.Second
)

// DockerConfigProvider implements the ConfigProvider interface for the docker labels.
type DockerConfigProvider struct {
	sync.RWMutex
	dockerUtil   *docker.DockerUtil
	upToDate     bool
	streaming    bool
	health       *health.Handle
	labelCache   map[string]map[string]string
	syncInterval int
	syncCounter  int
}

// NewDockerConfigProvider returns a new ConfigProvider connected to docker.
// Connectivity is not checked at this stage to allow for retries, Collect will do it.
func NewDockerConfigProvider(config config.ConfigurationProviders) (ConfigProvider, error) {
	return &DockerConfigProvider{
		// periodically resync every 30 runs if we're missing events
		syncInterval: 30,
	}, nil
}

// String returns a string representation of the DockerConfigProvider
func (d *DockerConfigProvider) String() string {
	return names.Docker
}

// Collect retrieves all running containers and extract AD templates from their labels.
func (d *DockerConfigProvider) Collect() ([]integration.Config, error) {
	var err error
	firstCollection := false

	d.Lock()
	if d.dockerUtil == nil {
		d.dockerUtil, err = docker.GetDockerUtil()
		if err != nil {
			d.Unlock()
			return []integration.Config{}, err
		}
		firstCollection = true
	}

	var containers map[string]map[string]string
	// on the first run we collect all labels, then rely on individual events to
	// avoid listing all containers too often
	if d.labelCache == nil || d.syncCounter == d.syncInterval {
		containers, err = d.dockerUtil.AllContainerLabels()
		if err != nil {
			d.Unlock()
			return []integration.Config{}, err
		}
		d.labelCache = containers
		d.syncCounter = 0
	} else {
		containers = d.labelCache
	}

	d.syncCounter++
	d.upToDate = true
	d.Unlock()

	// start listening after the first collection to avoid race in cache map init
	if firstCollection {
		go d.listen()
	}

	d.RLock()
	defer d.RUnlock()
	return parseDockerLabels(containers)
}

// We listen to docker events and invalidate our cache when we receive a start/die event
func (d *DockerConfigProvider) listen() {
	d.Lock()
	d.streaming = true
	d.health = health.RegisterLiveness("ad-dockerprovider")
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
				if ev.Action == "start" {
					container, err := d.dockerUtil.Inspect(ev.ContainerID, false)
					if err != nil {
						log.Warnf("Error inspecting container: %s", err)
					} else {
						d.Lock()
						_, containerSeen := d.labelCache[ev.ContainerID]
						d.Unlock()
						if containerSeen {
							// Container restarted with the same ID within 5 seconds.
							time.AfterFunc(delayDuration, func() {
								d.addLabels(ev.ContainerID, container.Config.Labels)
							})
						} else {
							d.addLabels(ev.ContainerID, container.Config.Labels)
						}
					}
				} else if ev.Action == "die" {
					// delay for short lived detection
					time.AfterFunc(delayDuration, func() {
						d.Lock()
						delete(d.labelCache, ev.ContainerID)
						d.upToDate = false
						d.Unlock()
					})
				}
			case err := <-errChan:
				log.Warnf("Error getting docker events: %s", err)
				d.Lock()
				d.upToDate = false
				d.Unlock()
				continue CONNECT // Re-connect to dockerutils
			}
		}
	}

	d.Lock()
	d.streaming = false
	d.health.Deregister() //nolint:errcheck
	d.Unlock()
}

// IsUpToDate checks whether we have new containers to parse, based on events received by the listen goroutine.
// If listening fails, we fallback to Collecting everytime.
func (d *DockerConfigProvider) IsUpToDate() (bool, error) {
	d.RLock()
	defer d.RUnlock()
	return (d.streaming && d.upToDate), nil
}

// addLabels updates the label cache for a given container
func (d *DockerConfigProvider) addLabels(containerID string, labels map[string]string) {
	d.Lock()
	defer d.Unlock()
	d.labelCache[containerID] = labels
	d.upToDate = false
}

func parseDockerLabels(containers map[string]map[string]string) ([]integration.Config, error) {
	var configs []integration.Config
	for cID, labels := range containers {
		dockerEntityName := docker.ContainerIDToEntityName(cID)
		c, errors := extractTemplatesFromMap(dockerEntityName, labels, dockerADLabelPrefix)

		for _, err := range errors {
			log.Errorf("Can't parse template for container %s: %s", cID, err)
		}

		for idx := range c {
			c[idx].Source = "docker:" + dockerEntityName
		}

		configs = append(configs, c...)
	}
	return configs, nil
}

func init() {
	RegisterProvider("docker", NewDockerConfigProvider)
}
