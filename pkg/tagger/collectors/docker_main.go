// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build docker

package collectors

import (
	"context"
	"io"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

const (
	dockerCollectorName = "docker"
)

// DockerCollector listens to events on the docker socket to get new/dead containers
// and feed a stram of TagInfo. It requires access to the docker socket.
// It will also embed DockerExtractor collectors for container tagging.
type DockerCollector struct {
	dockerUtil   *docker.DockerUtil
	stop         chan bool
	infoOut      chan<- []*TagInfo
	labelsAsTags map[string]string
	envAsTags    map[string]string
}

// Detect tries to connect to the docker socket and returns success
func (c *DockerCollector) Detect(_ context.Context, out chan<- []*TagInfo) (CollectionMode, error) {
	if !config.IsFeaturePresent(config.Docker) {
		return NoCollection, nil
	}

	du, err := docker.GetDockerUtil()
	if err != nil {
		return NoCollection, err
	}

	c.dockerUtil = du
	c.stop = make(chan bool)
	c.infoOut = out

	// We lower-case the values collected by viper as well as the ones from inspecting the labels of containers.
	c.labelsAsTags = retrieveMappingFromConfig("docker_labels_as_tags")
	c.envAsTags = retrieveMappingFromConfig("docker_env_as_tags")

	// TODO: list and inspect existing containers once docker utils are merged

	return StreamCollection, nil
}

// Stream runs the continuous event watching loop and sends new info
// to the channel. But be called in a goroutine.
func (c *DockerCollector) Stream() error {
	ctx, cancel := context.WithCancel(context.Background())

	health := health.RegisterLiveness("tagger-docker")

	messages, errs, err := c.dockerUtil.SubscribeToContainerEvents("DockerCollector")
	if err != nil {
		return err
	}

	for {
		select {
		case <-c.stop:
			health.Deregister() //nolint:errcheck
			cancel()
			return c.dockerUtil.UnsubscribeFromContainerEvents("DockerCollector")
		case healthDeadline := <-health.C:
			cancel()
			ctx, cancel = context.WithDeadline(context.Background(), healthDeadline)
		case msg := <-messages:
			c.processEvent(ctx, msg)
		case err := <-errs:
			if err != nil && err != io.EOF {
				log.Errorf("stopping collection: %s", err)
				cancel()
				return err
			}
			cancel()
			return nil
		}
	}
}

// Stop queues a shutdown of DockerListener
func (c *DockerCollector) Stop() error {
	c.stop <- true
	return nil
}

// Fetch inspect a given container to get its tags on-demand (cache miss)
func (c *DockerCollector) Fetch(ctx context.Context, entity string) ([]string, []string, []string, error) {
	entityType, cID := containers.SplitEntityName(entity)
	if entityType != containers.ContainerEntityName || len(cID) == 0 {
		return nil, nil, nil, nil
	}

	low, orchestrator, high, _, err := c.fetchForDockerID(ctx, cID, fetchOptions{
		inspectCached: false,
		skipExited:    true,
	})

	return low, orchestrator, high, err
}

func (c *DockerCollector) processEvent(ctx context.Context, e *docker.ContainerEvent) {
	var info *TagInfo

	switch e.Action {
	case docker.ContainerEventActionDie, docker.ContainerEventActionDied:
		info = &TagInfo{
			Entity:       e.ContainerEntityName(),
			Source:       dockerCollectorName,
			DeleteEntity: true,
		}
	case docker.ContainerEventActionStart, docker.ContainerEventActionRename:
		low, orchestrator, high, standard, err := c.fetchForDockerID(ctx, e.ContainerID, fetchOptions{
			inspectCached: e.Action == docker.ContainerEventActionStart,
		})
		if err != nil {
			log.Debugf("Error fetching tags for container '%s': %v", e.ContainerName, err)
		}
		info = &TagInfo{
			Entity:               e.ContainerEntityName(),
			Source:               dockerCollectorName,
			LowCardTags:          low,
			OrchestratorCardTags: orchestrator,
			HighCardTags:         high,
			StandardTags:         standard,
		}
	default:
		return // Nothing to see here
	}
	c.infoOut <- []*TagInfo{info}
}

type fetchOptions struct {
	inspectCached bool
	skipExited    bool
}

func (c *DockerCollector) fetchForDockerID(ctx context.Context, cID string, options fetchOptions) ([]string, []string, []string, []string, error) {
	var (
		co  types.ContainerJSON
		err error
	)

	if options.inspectCached {
		co, err = c.dockerUtil.InspectNoCache(ctx, cID, false)
	} else {
		co, err = c.dockerUtil.Inspect(ctx, cID, false)
	}

	if err != nil {
		if !errors.IsNotFound(err) {
			log.Debugf("Failed to inspect container %s - %s", cID, err)
		}
		return nil, nil, nil, nil, err
	}

	if options.skipExited && (co.State.Status == "exited" || co.State.Status == "died") {
		return nil, nil, nil, nil, nil
	}

	low, orchestrator, high, standard := c.extractFromInspect(co)
	return low, orchestrator, high, standard, nil
}

func dockerFactory() Collector {
	return &DockerCollector{}
}

func init() {
	registerCollector(dockerCollectorName, dockerFactory, NodeRuntime)
}
