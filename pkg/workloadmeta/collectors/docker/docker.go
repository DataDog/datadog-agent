// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package docker

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	collectorID   = "docker"
	componentName = "workloadmeta-docker"
)

type resolveHook func(ctx context.Context, co types.ContainerJSON) (string, error)

type collector struct {
	store workloadmeta.Store

	dockerUtil *docker.DockerUtil
	eventCh    <-chan *docker.ContainerEvent
	errCh      <-chan error
}

func init() {
	workloadmeta.RegisterCollector(collectorID, func() workloadmeta.Collector {
		return &collector{}
	})
}

func (c *collector) Start(ctx context.Context, store workloadmeta.Store) error {
	if !config.IsFeaturePresent(config.Docker) {
		return errors.NewDisabled(componentName, "Agent is not running on Docker")
	}

	c.store = store

	var err error
	c.dockerUtil, err = docker.GetDockerUtil()
	if err != nil {
		return err
	}

	filter, err := containers.GetPauseContainerFilter()
	if err != nil {
		log.Warnf("Can't get pause container filter, no filtering will be applied: %w", err)
	}

	c.eventCh, c.errCh, err = c.dockerUtil.SubscribeToContainerEvents(componentName, filter)
	if err != nil {
		return err
	}

	err = c.generateEventsFromContainerList(ctx)
	if err != nil {
		return err
	}

	go c.stream(ctx)

	return nil
}

func (c *collector) Pull(ctx context.Context) error {
	return nil
}

func (c *collector) stream(ctx context.Context) {
	health := health.RegisterLiveness(componentName)
	ctx, cancel := context.WithCancel(ctx)

	for {
		select {
		case <-health.C:

		case ev := <-c.eventCh:
			err := c.handleEvent(ctx, ev)
			if err != nil {
				log.Warnf(err.Error())
			}

		case err := <-c.errCh:
			if err != nil && err != io.EOF {
				log.Errorf("stopping collection: %s", err)
			}

			cancel()

			return

		case <-ctx.Done():
			var err error

			err = c.dockerUtil.UnsubscribeFromContainerEvents("DockerCollector")
			if err != nil {
				log.Warnf("error unsubscribbing from container events: %s", err)
			}

			err = health.Deregister()
			if err != nil {
				log.Warnf("error de-registering health check: %s", err)
			}

			cancel()

			return
		}
	}
}

func (c *collector) generateEventsFromContainerList(ctx context.Context) error {
	containers, err := c.dockerUtil.RawContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		return err
	}

	events := make([]workloadmeta.CollectorEvent, 0, len(containers))
	for _, container := range containers {
		ev, err := c.buildCollectorEvent(ctx, &docker.ContainerEvent{
			ContainerID: container.ID,
			Action:      docker.ContainerEventActionStart,
		})
		if err != nil {
			log.Warnf(err.Error())
		}

		events = append(events, ev)
	}

	if len(events) > 0 {
		c.store.Notify(events)
	}

	return nil
}

func (c *collector) handleEvent(ctx context.Context, ev *docker.ContainerEvent) error {
	event, err := c.buildCollectorEvent(ctx, ev)
	if err != nil {
		return err
	}

	c.store.Notify([]workloadmeta.CollectorEvent{event})

	return nil
}

func (c *collector) buildCollectorEvent(ctx context.Context, ev *docker.ContainerEvent) (workloadmeta.CollectorEvent, error) {
	event := workloadmeta.CollectorEvent{
		Source: workloadmeta.SourceDocker,
	}

	entityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindContainer,
		ID:   ev.ContainerID,
	}

	switch ev.Action {
	case docker.ContainerEventActionStart, docker.ContainerEventActionRename:
		container, err := c.dockerUtil.InspectNoCache(ctx, ev.ContainerID, false)
		if err != nil {
			return event, fmt.Errorf("could not inspect container %q: %s", ev.ContainerID, err)
		}

		var startedAt time.Time
		if container.State.StartedAt != "" {
			startedAt, err = time.Parse(time.RFC3339, container.State.StartedAt)
			if err != nil {
				log.Debugf("cannot parse StartedAt %q for container %q: %s", container.State.StartedAt, container.ID, err)
			}
		}

		var finishedAt time.Time
		if container.State.FinishedAt != "" {
			finishedAt, err = time.Parse(time.RFC3339, container.State.FinishedAt)
			if err != nil {
				log.Debugf("cannot parse FinishedAt %q for container %q: %s", container.State.FinishedAt, container.ID, err)
			}
		}

		event.Type = workloadmeta.EventTypeSet
		event.Entity = &workloadmeta.Container{
			EntityID: entityID,
			EntityMeta: workloadmeta.EntityMeta{
				Name:   strings.TrimPrefix(container.Name, "/"),
				Labels: container.Config.Labels,
			},
			Image:   extractImage(ctx, container, c.dockerUtil.ResolveImageNameFromContainer),
			EnvVars: extractEnvVars(container.Config.Env),
			Ports:   extractPorts(container),
			Runtime: workloadmeta.ContainerRuntimeDocker,
			State: workloadmeta.ContainerState{
				Running:    container.State.Running,
				StartedAt:  startedAt,
				FinishedAt: finishedAt,
			},
			NetworkIPs: extractNetworkIPs(container.NetworkSettings.Networks),
			Hostname:   container.Config.Hostname,
			PID:        container.State.Pid,
		}

	case docker.ContainerEventActionDie, docker.ContainerEventActionDied:
		event.Type = workloadmeta.EventTypeUnset
		event.Entity = entityID

	default:
		return event, fmt.Errorf("unknown action type %q, ignoring", ev.Action)
	}

	return event, nil
}

func extractImage(ctx context.Context, container types.ContainerJSON, resolve resolveHook) workloadmeta.ContainerImage {
	imageSpec := container.Config.Image
	image := workloadmeta.ContainerImage{
		RawName: imageSpec,
		Name:    imageSpec,
	}

	var (
		name      string
		shortName string
		tag       string
		err       error
	)

	if strings.Contains(imageSpec, "@sha256") {
		name, shortName, tag, err = containers.SplitImageName(imageSpec)
		if err != nil {
			log.Debugf("cannot split image name %q for container %q: %s", imageSpec, container.ID, err)
		}
	}

	if name == "" && tag == "" {
		resolvedImageSpec, err := resolve(ctx, container)
		if err != nil {
			log.Debugf("cannot resolve image name %q for container %q: %s", imageSpec, container.ID, err)
			return image
		}

		name, shortName, tag, err = containers.SplitImageName(resolvedImageSpec)
		if err != nil {
			log.Debugf("cannot split image name %q for container %q: %s", resolvedImageSpec, container.ID, err)

			// fallback and try to parse the original imageSpec anyway
			if err == containers.ErrImageIsSha256 {
				name, shortName, tag, err = containers.SplitImageName(imageSpec)
				if err != nil {
					log.Debugf("cannot split image name %q for container %q: %s", imageSpec, container.ID, err)
					return image
				}
			} else {
				return image
			}
		}
	}

	image.Name = name
	image.ShortName = shortName
	image.Tag = tag

	return image
}

func extractEnvVars(env []string) map[string]string {
	envMap := make(map[string]string)

	for _, e := range env {
		envSplit := strings.SplitN(e, "=", 2)
		if len(envSplit) != 2 {
			log.Debugf("cannot parse env var from string: %q", e)
			continue
		}

		envMap[envSplit[0]] = envSplit[1]
	}

	return envMap
}

func extractPorts(container types.ContainerJSON) []workloadmeta.ContainerPort {
	var ports []workloadmeta.ContainerPort

	// yes, the code in both branches is exactly the same. unfortunately.
	// Ports and ExposedPorts are different types.
	switch {
	case len(container.NetworkSettings.Ports) > 0:
		for p := range container.NetworkSettings.Ports {
			ports = append(ports, extractPort(p)...)
		}
	case len(container.Config.ExposedPorts) > 0:
		for p := range container.Config.ExposedPorts {
			ports = append(ports, extractPort(p)...)
		}
	}

	return ports
}

func extractPort(port nat.Port) []workloadmeta.ContainerPort {
	var output []workloadmeta.ContainerPort

	// Try to parse a port range, eg. 22-25
	first, last, err := port.Range()
	if err != nil {
		log.Debugf("cannot get port range from nat.Port: %s", err)
		return output
	}

	if last > first {
		output = make([]workloadmeta.ContainerPort, 0, last-first+1)
		for p := first; p <= last; p++ {
			output = append(output, workloadmeta.ContainerPort{
				Port:     p,
				Protocol: port.Proto(),
			})
		}

		return output
	}

	// Try to parse a single port (most common case)
	p := port.Int()
	if p > 0 {
		output = []workloadmeta.ContainerPort{
			{
				Port:     p,
				Protocol: port.Proto(),
			},
		}
	}

	return output
}

func extractNetworkIPs(networks map[string]*network.EndpointSettings) map[string]string {
	networkIPs := make(map[string]string)

	for net, settings := range networks {
		if len(settings.IPAddress) > 0 {
			networkIPs[net] = settings.IPAddress
		}
	}

	return networkIPs
}
