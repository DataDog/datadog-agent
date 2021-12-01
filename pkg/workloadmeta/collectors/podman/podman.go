// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build podman
// +build podman

package podman

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/podman"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/util"
)

const (
	collectorID   = "podman"
	componentName = "workloadmeta-podman"
	expireFreq    = 10 * time.Second
)

type podmanClient interface {
	GetAllContainers() ([]podman.Container, error)
}

type collector struct {
	client podmanClient
	store  workloadmeta.Store
	expire *util.Expire
}

func init() {
	workloadmeta.RegisterCollector(collectorID, func() workloadmeta.Collector {
		return &collector{}
	})
}

func (c *collector) Start(_ context.Context, store workloadmeta.Store) error {
	if !config.IsFeaturePresent(config.Podman) {
		return dderrors.NewDisabled(componentName, "Podman not detected")
	}

	c.client = podman.NewDBClient(podman.DefaultDBPath)
	c.store = store
	c.expire = util.NewExpire(expireFreq)

	return nil
}

func (c *collector) Pull(_ context.Context) error {
	containers, err := c.client.GetAllContainers()
	if err != nil {
		return err
	}

	var events []workloadmeta.CollectorEvent

	for _, container := range containers {
		event := convertToEvent(&container)
		c.expire.Update(event.Entity.GetID(), time.Now())
		events = append(events, event)
	}

	events = append(events, c.expiredEvents()...)

	c.store.Notify(events)

	return nil
}

func convertToEvent(container *podman.Container) workloadmeta.CollectorEvent {
	containerID := container.Config.ID

	var annotations map[string]string
	if spec := container.Config.Spec; spec != nil {
		annotations = spec.Annotations
	}

	envs, err := envVars(container)
	if err != nil {
		log.Warnf("Could not get env vars for container %s", containerID)
	}

	image, err := workloadmeta.NewContainerImage(container.Config.RawImageName)
	if err != nil {
		log.Warnf("Could not get image for container %s", containerID)
	}

	var ports []workloadmeta.ContainerPort
	for _, portMapping := range container.Config.PortMappings {
		ports = append(ports, workloadmeta.ContainerPort{
			Port:     int(portMapping.ContainerPort),
			Protocol: portMapping.Protocol,
		})
	}

	return workloadmeta.CollectorEvent{
		Type:   workloadmeta.EventTypeSet,
		Source: workloadmeta.SourcePodman,
		Entity: &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   containerID,
			},
			EntityMeta: workloadmeta.EntityMeta{
				Name:        container.Config.Name,
				Namespace:   container.Config.Namespace,
				Annotations: annotations,
				Labels:      container.Config.Labels,
			},
			EnvVars:    envs,
			Hostname:   hostname(container),
			Image:      image,
			NetworkIPs: make(map[string]string), // I think there's no way to get this mapping
			PID:        container.State.PID,
			Ports:      ports,
			Runtime:    workloadmeta.ContainerRuntimePodman,
			State: workloadmeta.ContainerState{
				Running:    container.State.State == podman.ContainerStateRunning,
				StartedAt:  container.State.StartedTime,
				FinishedAt: container.State.FinishedTime,
			},
		},
	}
}

func (c *collector) expiredEvents() []workloadmeta.CollectorEvent {
	var res []workloadmeta.CollectorEvent

	for _, expired := range c.expire.ComputeExpires() {
		res = append(res, workloadmeta.CollectorEvent{
			Type:   workloadmeta.EventTypeUnset,
			Source: workloadmeta.SourcePodman,
			Entity: expired,
		})
	}

	return res
}

func envVars(container *podman.Container) (map[string]string, error) {
	res := make(map[string]string)

	if container.Config.Spec == nil || container.Config.Spec.Process == nil {
		return res, nil
	}

	for _, env := range container.Config.Spec.Process.Env {
		envSplit := strings.SplitN(env, "=", 2)

		if len(envSplit) < 2 {
			return nil, errors.New("unexpected environment variable format")
		}

		res[envSplit[0]] = envSplit[1]
	}

	return res, nil
}

// This function has been copied from
// https://github.com/containers/podman/blob/v3.4.1/libpod/container.go
func hostname(container *podman.Container) string {
	if container.Config.Spec.Hostname != "" {
		return container.Config.Spec.Hostname
	}

	if len(container.Config.ID) < 11 {
		return container.Config.ID
	}
	return container.Config.ID[:12]
}
