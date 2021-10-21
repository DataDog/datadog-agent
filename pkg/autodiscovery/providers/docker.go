// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package providers

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	dockerADLabelPrefix = "com.datadoghq.ad."
	delayDuration       = 5 * time.Second
)

// DockerConfigProvider implements the ConfigProvider interface for the docker labels.
type DockerConfigProvider struct {
	sync.RWMutex
	workloadmetaStore workloadmeta.Store
	upToDate          bool
	streaming         bool
	labelCache        map[string]map[string]string
	containerFilter   *containers.Filter
	once              sync.Once
}

func NewDockerConfigProvider(config config.ConfigurationProviders) (ConfigProvider, error) {
	containerFilter, err := containers.NewAutodiscoveryFilter(containers.GlobalFilter)
	if err != nil {
		log.Warnf("Can't get container include/exclude filter, no filtering will be applied: %w", err)
	}

	return &DockerConfigProvider{
		workloadmetaStore: workloadmeta.GetGlobalStore(),
		labelCache:        make(map[string]map[string]string),
		containerFilter:   containerFilter,
	}, nil
}

// String returns a string representation of the DockerConfigProvider
func (d *DockerConfigProvider) String() string {
	return names.Docker
}

// Collect retrieves all running containers and extract AD templates from their labels.
func (d *DockerConfigProvider) Collect(ctx context.Context) ([]integration.Config, error) {
	d.once.Do(func() {
		go d.listen()
	})

	d.Lock()
	d.upToDate = true
	d.Unlock()

	d.RLock()
	defer d.RUnlock()
	return parseDockerLabels(d.labelCache)
}

func (d *DockerConfigProvider) listen() {
	d.Lock()
	d.streaming = true
	health := health.RegisterLiveness("ad-dockerprovider")
	d.Unlock()

	workloadmetaEventsChannel := d.workloadmetaStore.Subscribe("ad-dockerprovider", workloadmeta.NewFilter(
		[]workloadmeta.Kind{workloadmeta.KindContainer},
		[]string{"docker"},
	))

	for {
		select {
		case evBundle := <-workloadmetaEventsChannel:
			d.processEvents(evBundle)

		case <-health.C:

		}
	}
}

func (d *DockerConfigProvider) processEvents(eventBundle workloadmeta.EventBundle) {
	close(eventBundle.Ch)

	for _, event := range eventBundle.Events {
		containerID := event.Entity.GetID().ID

		switch event.Type {
		case workloadmeta.EventTypeSet:
			container := event.Entity.(*workloadmeta.Container)

			d.RLock()
			_, containerSeen := d.labelCache[container.ID]
			d.RUnlock()
			if containerSeen {
				// Container restarted with the same ID within 5 seconds.
				// This delay is needed because of the delay introduced in the
				// EventTypeUnset case.
				time.AfterFunc(delayDuration, func() {
					d.addLabels(containerID, container.Labels)
				})
			} else {
				d.addLabels(containerID, container.Labels)
			}

		case workloadmeta.EventTypeUnset:
			// delay for short-lived detection
			time.AfterFunc(delayDuration, func() {
				d.deleteLabels(containerID)
			})

		default:
			log.Errorf("cannot handle event of type %d", event.Type)
		}
	}

}

// IsUpToDate checks whether we have new containers to parse, based on events received by the listen goroutine.
// If listening fails, we fallback to Collecting everytime.
func (d *DockerConfigProvider) IsUpToDate(ctx context.Context) (bool, error) {
	d.RLock()
	defer d.RUnlock()
	return d.streaming && d.upToDate, nil
}

// addLabels updates the label cache for a given container
func (d *DockerConfigProvider) addLabels(containerID string, labels map[string]string) {
	d.Lock()
	defer d.Unlock()
	d.labelCache[containerID] = labels
	d.upToDate = false
}

func (d *DockerConfigProvider) deleteLabels(containerID string) {
	d.Lock()
	defer d.Unlock()
	delete(d.labelCache, containerID)
	d.upToDate = false
}

func parseDockerLabels(containerLabels map[string]map[string]string) ([]integration.Config, error) {
	var configs []integration.Config
	for containerID, labels := range containerLabels {
		dockerEntityName := docker.ContainerIDToEntityName(containerID)
		c, errors := extractTemplatesFromMap(dockerEntityName, labels, dockerADLabelPrefix)

		for _, err := range errors {
			log.Errorf("Can't parse template for container %s: %s", containerID, err)
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

// GetConfigErrors is not implemented for the DockerConfigProvider
func (d *DockerConfigProvider) GetConfigErrors() map[string]ErrorMsgSet {
	return make(map[string]ErrorMsgSet)
}
