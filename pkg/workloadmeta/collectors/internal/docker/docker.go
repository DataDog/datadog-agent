// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/scanner"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	collectorID   = "docker"
	componentName = "workloadmeta-docker"
)

type resolveHook func(ctx context.Context, co types.ContainerJSON) (string, error)

type collector struct {
	store workloadmeta.Store

	dockerUtil        *docker.DockerUtil
	containerEventsCh <-chan *docker.ContainerEvent
	imageEventsCh     <-chan *docker.ImageEvent

	// Images are updated from 2 goroutines: the one that handles docker
	// events, and the one that extracts SBOMS.
	// This mutex is used to handle images one at a time to avoid
	// inconsistencies like trying to set an SBOM for an image that is being
	// deleted.
	handleImagesMut sync.Mutex

	// SBOM Scanning
	sbomScanner *scanner.Scanner // nolint: unused
	scanOptions sbom.ScanOptions // nolint: unused
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

	if err = c.startSBOMCollection(ctx); err != nil {
		return err
	}

	filter, err := containers.GetPauseContainerFilter()
	if err != nil {
		log.Warnf("Can't get pause container filter, no filtering will be applied: %v", err)
	}

	c.containerEventsCh, c.imageEventsCh, err = c.dockerUtil.SubscribeToEvents(componentName, filter)
	if err != nil {
		return err
	}

	err = c.generateEventsFromContainerList(ctx, filter)
	if err != nil {
		return err
	}

	err = c.generateEventsFromImageList(ctx)
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

		case ev := <-c.containerEventsCh:
			err := c.handleContainerEvent(ctx, ev)
			if err != nil {
				log.Warnf(err.Error())
			}

		case ev := <-c.imageEventsCh:
			err := c.handleImageEvent(ctx, ev, nil)
			if err != nil {
				log.Warnf(err.Error())
			}

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

func (c *collector) generateEventsFromContainerList(ctx context.Context, filter *containers.Filter) error {
	containers, err := c.dockerUtil.RawContainerListWithFilter(ctx, types.ContainerListOptions{}, filter)
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
			continue
		}

		events = append(events, ev)
	}

	if len(events) > 0 {
		c.store.Notify(events)
	}

	return nil
}

func (c *collector) generateEventsFromImageList(ctx context.Context) error {
	images, err := c.dockerUtil.Images(ctx, true)
	if err != nil {
		return err
	}

	events := make([]workloadmeta.CollectorEvent, 0, len(images))

	for _, img := range images {
		imgMetadata, err := c.getImageMetadata(ctx, img.ID, nil)
		if err != nil {
			log.Warnf(err.Error())
			continue
		}

		event := workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceRuntime,
			Type:   workloadmeta.EventTypeSet,
			Entity: imgMetadata,
		}

		events = append(events, event)
	}

	if len(events) > 0 {
		c.store.Notify(events)
	}

	return nil
}

func (c *collector) handleContainerEvent(ctx context.Context, ev *docker.ContainerEvent) error {
	event, err := c.buildCollectorEvent(ctx, ev)
	if err != nil {
		return err
	}

	c.store.Notify([]workloadmeta.CollectorEvent{event})

	return nil
}

func (c *collector) buildCollectorEvent(ctx context.Context, ev *docker.ContainerEvent) (workloadmeta.CollectorEvent, error) {
	event := workloadmeta.CollectorEvent{
		Source: workloadmeta.SourceRuntime,
	}

	entityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindContainer,
		ID:   ev.ContainerID,
	}

	switch ev.Action {
	case docker.ContainerEventActionStart,
		docker.ContainerEventActionRename,
		docker.ContainerEventActionHealthStatus:

		container, err := c.dockerUtil.InspectNoCache(ctx, ev.ContainerID, false)
		if err != nil {
			return event, fmt.Errorf("could not inspect container %q: %s", ev.ContainerID, err)
		}

		if ev.Action != docker.ContainerEventActionStart && !container.State.Running {
			return event, fmt.Errorf("received event: %s on dead container: %q, discarding", ev.Action, ev.ContainerID)
		}

		var createdAt time.Time
		if container.Created != "" {
			createdAt, err = time.Parse(time.RFC3339, container.Created)
			if err != nil {
				log.Debugf("Could not parse creation time '%q' for container %q: %s", container.Created, container.ID, err)
			}
		}

		var startedAt time.Time
		if container.State.StartedAt != "" {
			startedAt, err = time.Parse(time.RFC3339, container.State.StartedAt)
			if err != nil {
				log.Debugf("Cannot parse StartedAt %q for container %q: %s", container.State.StartedAt, container.ID, err)
			}
		}

		var finishedAt time.Time
		if container.State.FinishedAt != "" {
			finishedAt, err = time.Parse(time.RFC3339, container.State.FinishedAt)
			if err != nil {
				log.Debugf("Cannot parse FinishedAt %q for container %q: %s", container.State.FinishedAt, container.ID, err)
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
				Status:     extractStatus(container.State),
				Health:     extractHealth(container.State.Health),
				StartedAt:  startedAt,
				FinishedAt: finishedAt,
				CreatedAt:  createdAt,
			},
			NetworkIPs: extractNetworkIPs(container.NetworkSettings.Networks),
			Hostname:   container.Config.Hostname,
			PID:        container.State.Pid,
		}

	case docker.ContainerEventActionDie, docker.ContainerEventActionDied:
		var exitCode *uint32
		if exitCodeString, found := ev.Attributes["exitCode"]; found {
			exitCodeInt, err := strconv.ParseInt(exitCodeString, 10, 32)
			if err != nil {
				log.Debugf("Cannot convert exit code %q: %v", exitCodeString, err)
			} else {
				exitCode = pointer.Ptr(uint32(exitCodeInt))
			}
		}

		event.Type = workloadmeta.EventTypeUnset
		event.Entity = &workloadmeta.Container{
			EntityID: entityID,
			State: workloadmeta.ContainerState{
				Running:    false,
				FinishedAt: ev.Timestamp,
				ExitCode:   exitCode,
			},
		}

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
		registry  string
		shortName string
		tag       string
		err       error
	)

	if strings.Contains(imageSpec, "@sha256") {
		name, registry, shortName, tag, err = containers.SplitImageName(imageSpec)
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

		name, registry, shortName, tag, err = containers.SplitImageName(resolvedImageSpec)
		if err != nil {
			log.Debugf("cannot split image name %q for container %q: %s", resolvedImageSpec, container.ID, err)

			// fallback and try to parse the original imageSpec anyway
			if err == containers.ErrImageIsSha256 {
				name, registry, shortName, tag, err = containers.SplitImageName(imageSpec)
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
	image.Registry = registry
	image.ShortName = shortName
	image.Tag = tag
	image.ID = container.Image
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

		if containers.EnvVarFilterFromConfig().IsIncluded(envSplit[0]) {
			envMap[envSplit[0]] = envSplit[1]
		}
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

func extractStatus(containerState *types.ContainerState) workloadmeta.ContainerStatus {
	if containerState == nil {
		return workloadmeta.ContainerStatusUnknown
	}

	switch containerState.Status {
	case "created":
		return workloadmeta.ContainerStatusCreated
	case "running":
		return workloadmeta.ContainerStatusRunning
	case "paused":
		return workloadmeta.ContainerStatusPaused
	case "restarting":
		return workloadmeta.ContainerStatusRestarting
	case "removing", "exited", "dead":
		return workloadmeta.ContainerStatusStopped
	}

	return workloadmeta.ContainerStatusUnknown
}

func extractHealth(containerHealth *types.Health) workloadmeta.ContainerHealth {
	if containerHealth == nil {
		return workloadmeta.ContainerHealthUnknown
	}

	switch containerHealth.Status {
	case types.NoHealthcheck, types.Starting:
		return workloadmeta.ContainerHealthUnknown
	case types.Healthy:
		return workloadmeta.ContainerHealthHealthy
	case types.Unhealthy:
		return workloadmeta.ContainerHealthUnhealthy
	}

	return workloadmeta.ContainerHealthUnknown
}

func (c *collector) handleImageEvent(ctx context.Context, event *docker.ImageEvent, bom *workloadmeta.SBOM) error {
	c.handleImagesMut.Lock()
	defer c.handleImagesMut.Unlock()

	switch event.Action {
	case docker.ImageEventActionPull, docker.ImageEventActionTag, docker.ImageEventActionUntag, docker.ImageEventActionSbom:
		imgMetadata, err := c.getImageMetadata(ctx, event.ImageID, bom)
		if err != nil {
			return fmt.Errorf("could not get image metadata for image %q: %w", event.ImageID, err)
		}

		workloadmetaEvent := workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceRuntime,
			Type:   workloadmeta.EventTypeSet,
			Entity: imgMetadata,
		}

		c.store.Notify([]workloadmeta.CollectorEvent{workloadmetaEvent})
	case docker.ImageEventActionDelete:
		workloadmetaEvent := workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceRuntime,
			Type:   workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.ContainerImageMetadata{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainerImageMetadata,
					ID:   event.ImageID,
				},
			},
		}

		c.store.Notify([]workloadmeta.CollectorEvent{workloadmetaEvent})
	}

	return nil
}

func (c *collector) getImageMetadata(ctx context.Context, imageID string, bom *workloadmeta.SBOM) (*workloadmeta.ContainerImageMetadata, error) {
	imgInspect, err := c.dockerUtil.ImageInspect(ctx, imageID)
	if err != nil {
		return nil, err
	}

	imageHistory, err := c.dockerUtil.ImageHistory(ctx, imageID)
	if err != nil {
		// Not sure if it's possible to get the image history in all the
		// environments. If it's not, return the rest of metadata instead of an
		// error.
		log.Warnf("error getting image history: %s", err)
	}

	labels := make(map[string]string)
	if imgInspect.Config != nil {
		labels = imgInspect.Config.Labels
	}

	imageName := c.dockerUtil.GetPreferredImageName(
		imgInspect.ID,
		imgInspect.RepoTags,
		imgInspect.RepoDigests,
	)

	existingBOM := bom
	// We can get "create" events for images that already exist. That happens
	// when the same image is referenced with different names. For example,
	// datadog/agent:latest and datadog/agent:7 might refer to the same image.
	// Also, in some environments (at least with Kind), pulling an image like
	// datadog/agent:latest creates several events: in one of them the image
	// name is a digest, in other is something with the same format as
	// datadog/agent:7, and sometimes there's a temporary name prefixed with
	// "import-".
	// When that happens, give precedence to the name with repo and tag instead
	// of the name that includes a digest. This is just to show names that are
	// more user-friendly (the digests are already present in other attributes
	// like ID, and repo digest).
	existingImg, err := c.store.GetImage(imageID)
	if err == nil {
		if strings.Contains(imageName, "sha256:") && !strings.Contains(existingImg.Name, "sha256:") {
			imageName = existingImg.Name
		}

		if existingBOM == nil && existingImg.SBOM != nil {
			existingBOM = existingImg.SBOM
		}
	}

	return &workloadmeta.ContainerImageMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainerImageMetadata,
			ID:   imgInspect.ID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:   imageName,
			Labels: labels,
		},
		RepoTags:     imgInspect.RepoTags,
		RepoDigests:  imgInspect.RepoDigests,
		SizeBytes:    imgInspect.Size,
		OS:           imgInspect.Os,
		OSVersion:    imgInspect.OsVersion,
		Architecture: imgInspect.Architecture,
		Variant:      imgInspect.Variant,
		Layers:       layersFromDockerHistory(imageHistory),
		SBOM:         existingBOM,
	}, nil
}

func layersFromDockerHistory(history []image.HistoryResponseItem) []workloadmeta.ContainerImageLayer {
	var layers []workloadmeta.ContainerImageLayer

	// Docker returns the layers in reverse-chronological order
	for i := len(history) - 1; i >= 0; i-- {
		created := time.Unix(history[i].Created, 0)

		layer := workloadmeta.ContainerImageLayer{
			Digest:    history[i].ID,
			SizeBytes: history[i].Size,
			History: v1.History{
				Created:    &created,
				CreatedBy:  history[i].CreatedBy,
				Comment:    history[i].Comment,
				EmptyLayer: history[i].Size == 0,
			},
		}

		layers = append(layers, layer)
	}

	return layers
}
