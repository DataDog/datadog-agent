// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build containerd

package collectors

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/containerd/containerd"
	apievents "github.com/containerd/containerd/api/events"
	containerdevents "github.com/containerd/containerd/events"
	"github.com/gobwas/glob"
	"github.com/gogo/protobuf/proto"
)

const (
	containerdCollectorName = "containerd"
	containerCreationTopic  = "/containers/create"
	containerUpdateTopic    = "/containers/update"
	containerDeletionTopic  = "/containers/delete"
)

// containerLifecycleFilters allows subscribing to containers lifecycle updates only.
var containerLifecycleFilters = []string{
	fmt.Sprintf(`topic==%q`, containerCreationTopic),
	fmt.Sprintf(`topic==%q`, containerUpdateTopic),
	fmt.Sprintf(`topic==%q`, containerDeletionTopic),
}

// ContainderdCollector listens to events on the containerd socket and gets containers lifecycle updates.
// ContainderdCollector implements the Collector, Streamer and Fetcher interfaces.
type ContainderdCollector struct {
	client    cutil.ContainerdItf
	envAsTags map[string]string
	globEnv   map[string]glob.Glob
	infoOut   chan<- []*TagInfo
	stop      chan struct{}
}

var (
	_ Collector = &ContainderdCollector{}
	_ Streamer  = &ContainderdCollector{}
	_ Fetcher   = &ContainderdCollector{}
)

// Detect tries to connect to the containerd socket and initializes the ContainderdCollector attributes.
// Fast return if containerd wasn't detected by env discovery.
func (c *ContainderdCollector) Detect(_ context.Context, out chan<- []*TagInfo) (CollectionMode, error) {
	if !config.IsFeaturePresent(config.Containerd) {
		return NoCollection, nil
	}

	client, err := cutil.GetContainerdUtil()
	if err != nil {
		return NoCollection, err
	}

	c.client = client
	c.infoOut = out
	c.stop = make(chan struct{})
	c.envAsTags, c.globEnv = utils.InitMetadataAsTags(config.Datadog.GetStringMapString("container_env_as_tags"))

	return StreamCollection, nil
}

// Stream watches and processes containers lifecycle events.
func (c *ContainderdCollector) Stream() error {
	healthCtx, healthCancel := context.WithCancel(context.Background())
	health := health.RegisterLiveness("tagger-containerd")

	streamCtx, streamCancel := context.WithCancel(context.Background())
	stream, streamErrors := c.client.GetEvents().Subscribe(streamCtx, containerLifecycleFilters...)

	for {
		select {
		case <-c.stop:
			healthCancel()
			streamCancel()
			return health.Deregister()
		case healthDeadline := <-health.C:
			healthCancel()
			healthCtx, healthCancel = context.WithDeadline(context.Background(), healthDeadline)
		case event := <-stream:
			c.processEvent(healthCtx, event)
		case err := <-streamErrors:
			if err == nil {
				continue
			}

			log.Errorf("Containerd events stream error: %w", err)
			healthCancel()
			streamCancel()

			return err
		}
	}
}

// Fetch tries to get tags for a specific container by its ID.
func (c *ContainderdCollector) Fetch(ctx context.Context, entity string) ([]string, []string, []string, error) {
	entityType, id := containers.SplitEntityName(entity)
	if entityType != containers.ContainerEntityName || len(id) == 0 {
		return nil, nil, nil, nil
	}

	container, isPauseContainer, err := c.containerByID(ctx, id)
	if err != nil {
		log.Debugf("Could not fetch container %s - %w", id, err)
		return nil, nil, nil, err
	}

	if isPauseContainer {
		log.Debugf("Ignoring pause container %s", id)
		return nil, nil, nil, nil
	}

	low, orchestrator, high, _, err := c.tagsForContainer(ctx, container)
	return low, orchestrator, high, err
}

// Stop shut down the container events watching loop.
func (c *ContainderdCollector) Stop() error {
	close(c.stop)
	return nil
}

// processEvent handles container creation/update/deletion events and sends a TagInfo accordingly.
func (c *ContainderdCollector) processEvent(ctx context.Context, event *containerdevents.Envelope) {
	var handleCreateUpdate = func(id string) {
		container, isPauseContainer, err := c.containerByID(ctx, id)
		if err != nil {
			log.Debugf("Could not fetch container %q: %w", id, err)
			return
		}

		if isPauseContainer {
			log.Debugf("Ignoring pause container %q", id)
			return
		}

		low, orchestrator, high, standard, err := c.tagsForContainer(ctx, container)
		if err != nil {
			log.Debugf("Error fetching tags for container %q: %w", id, err)
			return
		}

		c.infoOut <- []*TagInfo{{
			Entity:               containers.BuildTaggerEntityName(id),
			Source:               containerdCollectorName,
			LowCardTags:          low,
			OrchestratorCardTags: orchestrator,
			HighCardTags:         high,
			StandardTags:         standard,
		}}
	}

	switch event.Topic {
	case containerCreationTopic:
		created := &apievents.ContainerCreate{}
		if err := proto.Unmarshal(event.Event.Value, created); err != nil {
			log.Debugf("Could not process container creation event: %w", err)
			return
		}

		log.Tracef("Container %q created", created.ID)
		handleCreateUpdate(created.ID)
	case containerUpdateTopic:
		updated := &apievents.ContainerUpdate{}
		if err := proto.Unmarshal(event.Event.Value, updated); err != nil {
			log.Debugf("Could not process container update event: %w", err)
			return
		}

		log.Tracef("Container %q updated", updated.ID)
		handleCreateUpdate(updated.ID)
	case containerDeletionTopic:
		deleted := &apievents.ContainerDelete{}
		if err := proto.Unmarshal(event.Event.Value, deleted); err != nil {
			log.Warnf("Could not process container deletion event: %w", err)
			return
		}

		log.Tracef("Container %q deleted", deleted.ID)
		c.infoOut <- []*TagInfo{{
			Entity:       containers.BuildTaggerEntityName(deleted.ID),
			Source:       containerdCollectorName,
			DeleteEntity: true,
		}}
	default:
		log.Debugf("Unsupported event topic: %s", event.Topic)
		return
	}
}

// containerByID fetches a containerd.Container by ID.
func (c *ContainderdCollector) containerByID(ctx context.Context, id string) (containerd.Container, bool, error) {
	container, err := c.client.ContainerWithContext(ctx, id)
	if err != nil {
		return nil, false, err
	}

	labels, err := c.client.LabelsWithContext(ctx, container)
	if err != nil {
		return nil, false, err
	}

	return container, containers.IsPauseContainer(labels), nil
}

// tagsForContainer extracts tas for a given container.
func (c *ContainderdCollector) tagsForContainer(ctx context.Context, container containerd.Container) ([]string, []string, []string, []string, error) {
	containerSpec, err := c.client.SpecWithContext(ctx, container)
	if err != nil {
		log.Debugf("Could not get container spec %s - %v", container.ID(), err)
		return nil, nil, nil, nil, err
	}

	low, orchestrator, high, standard := c.extractTags(containerSpec)
	return low, orchestrator, high, standard, nil
}

func containerdFactory() Collector {
	return &ContainderdCollector{}
}

func init() {
	registerCollector(containerdCollectorName, containerdFactory, NodeRuntime)
}
