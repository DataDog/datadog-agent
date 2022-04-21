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
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/podman"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/util"
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

func (c *collector) Start(ctx context.Context, store workloadmeta.Store) error {
	if !config.IsFeaturePresent(config.Podman) {
		return dderrors.NewDisabled(componentName, "Podman not detected")
	}

	c.client = podman.NewDBClient(podman.DefaultDBPath)
	c.store = store
	c.expire = util.NewExpire(expireFreq)

	err := c.Pull(ctx)
	if err != nil {
		return &retry.Error{
			LogicError:    err,
			RessourceName: collectorID,
			RetryStatus:   retry.FailWillRetry,
		}
	}

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

	var eventType workloadmeta.EventType
	if container.State.State == podman.ContainerStateRunning {
		eventType = workloadmeta.EventTypeSet
	} else {
		eventType = workloadmeta.EventTypeUnset
	}

	return workloadmeta.CollectorEvent{
		Type:   eventType,
		Source: workloadmeta.SourceRuntime,
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
			NetworkIPs: networkIPs(container),
			PID:        container.State.PID,
			Ports:      ports,
			Runtime:    workloadmeta.ContainerRuntimePodman,
			State: workloadmeta.ContainerState{
				Running:    container.State.State == podman.ContainerStateRunning,
				Status:     status(container.State.State),
				StartedAt:  container.State.StartedTime,
				CreatedAt:  container.State.StartedTime, // CreatedAt not available
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
			Source: workloadmeta.SourceRuntime,
			Entity: &workloadmeta.Container{
				EntityID: expired,
			},
		})
	}

	return res
}

func getShortID(container *podman.Container) (containerID string) {
	if len(container.Config.ID) >= 12 {
		containerID = container.Config.ID[:12]
	} else {
		containerID = container.Config.ID
	}
	return
}

func networkIPs(container *podman.Container) map[string]string {
	res := make(map[string]string)

	// container.Config.Networks contains only the networks specified at container creation time
	// and not the ones attached afterwards with `podman network attach`
	// They appear in the order in which they were specified in the `podman run --net=…` command
	networkNames := make([]string, len(container.Config.Networks))
	copy(networkNames, container.Config.Networks)
	sort.Strings(networkNames)

	// Handle the default case where no `--net` is specified
	if len(networkNames) == 0 && len(container.State.NetworkStatus) == 1 {
		networkNames = []string{"podman"}
	}

	if len(networkNames) != len(container.State.NetworkStatus) {
		log.Warnf("podman container %s %s has now a number of networks (%d) different from what it was at creation time (%d). This can be due to the use of `podman network attach`/`podman network detach`. This may confuse the agent.", getShortID(container), container.Config.Name, len(container.State.NetworkStatus), len(networkNames))
		return map[string]string{}
	}

	// container.State.NetworkStatus contains all the networks but they are not in the same order
	// as in container.Config.Network. Here, they are sorted by network name.
	for i := 0; i < len(networkNames); i++ {
		if len(container.State.NetworkStatus[i].IPs) > 1 {
			log.Warnf("podman container %s %s has several IPs on network %s. This is most probably because of a dual-stack IPv4/IPv6 setup. The agent will use only the first IP.", getShortID(container), container.Config.Name, networkNames[i])
		}
		if len(container.State.NetworkStatus[i].IPs) > 0 {
			res[networkNames[i]] = container.State.NetworkStatus[i].IPs[0].Address.IP.String()
		}
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

func status(state podman.ContainerStatus) workloadmeta.ContainerStatus {
	switch state {
	case podman.ContainerStateConfigured, podman.ContainerStateCreated:
		return workloadmeta.ContainerStatusCreated
	case podman.ContainerStateStopping, podman.ContainerStateExited, podman.ContainerStateStopped, podman.ContainerStateRemoving:
		return workloadmeta.ContainerStatusStopped
	case podman.ContainerStateRunning:
		return workloadmeta.ContainerStatusRunning
	case podman.ContainerStatePaused:
		return workloadmeta.ContainerStatusPaused
	}

	return workloadmeta.ContainerStatusUnknown
}
