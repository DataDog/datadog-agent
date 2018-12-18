// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package collectors

import (
	"io"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"

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
func (c *DockerCollector) Detect(out chan<- []*TagInfo) (CollectionMode, error) {
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
	healthHandle := health.Register("tagger-docker")

	messages, errs, err := c.dockerUtil.SubscribeToContainerEvents("DockerCollector")
	if err != nil {
		return err
	}

	for {
		select {
		case <-c.stop:
			healthHandle.Deregister()
			return c.dockerUtil.UnsubscribeFromContainerEvents("DockerCollector")
		case <-healthHandle.C:
		case msg := <-messages:
			c.processEvent(msg)
		case err := <-errs:
			if err != nil && err != io.EOF {
				log.Errorf("stopping collection: %s", err)
				return err
			}
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
func (c *DockerCollector) Fetch(entity string) ([]string, []string, error) {
	runtime, cID := containers.SplitEntityName(entity)
	if runtime != containers.RuntimeNameDocker || len(cID) == 0 {
		return nil, nil, nil
	}
	return c.fetchForDockerID(cID)
}

func (c *DockerCollector) processEvent(e *docker.ContainerEvent) {
	var info *TagInfo

	switch e.Action {
	case "die":
		info = &TagInfo{Entity: e.ContainerEntityName(), Source: dockerCollectorName, DeleteEntity: true}
	case "start":
		low, high, _ := c.fetchForDockerID(e.ContainerID)
		info = &TagInfo{Entity: e.ContainerEntityName(), Source: dockerCollectorName, LowCardTags: low, HighCardTags: high}
	default:
		return // Nothing to see here
	}
	c.infoOut <- []*TagInfo{info}
}

func (c *DockerCollector) fetchForDockerID(cID string) ([]string, []string, error) {
	co, err := c.dockerUtil.Inspect(cID, false)
	if err != nil {
		// TODO separate "not found" and inspect error
		log.Debugf("Failed to inspect container %s - %s", cID, err)
		return nil, nil, err
	}
	return c.extractFromInspect(co)
}

func dockerFactory() Collector {
	return &DockerCollector{}
}

func init() {
	registerCollector(dockerCollectorName, dockerFactory, NodeRuntime)
}
