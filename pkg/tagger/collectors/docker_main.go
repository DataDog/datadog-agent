// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package collectors

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	log "github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

const (
	dockerCollectorName = "docker"
)

// DockerCollector listens to events on the docker socket to get new/dead containers
// and feed a stram of TagInfo. It requires access to the docker socket.
// It will also embed DockerExtractor collectors for container tagging.
type DockerCollector struct {
	client       *client.Client
	stop         chan bool
	infoOut      chan<- []*TagInfo
	labelsAsTags map[string]string
	envAsTags    map[string]string
}

// Detect tries to connect to the docker socket and returns success
func (c *DockerCollector) Detect(out chan<- []*TagInfo) (CollectionMode, error) {
	// TODO: refactor with collector.listeners.DockerListener
	client, err := docker.ConnectToDocker()
	if err != nil {
		return NoCollection, fmt.Errorf("Failed to connect to Docker, docker tagging will not work: %s", err)
	}
	c.client = client
	c.stop = make(chan bool)
	c.infoOut = out

	// viper lower-cases map keys, so extractor must lowercase before matching
	c.labelsAsTags = config.Datadog.GetStringMapString("docker_labels_as_tags")
	c.envAsTags = config.Datadog.GetStringMapString("docker_env_as_tags")

	// TODO: list and inspect existing containers once docker utils are merged

	return StreamCollection, nil
}

// Stream runs the continuous event watching loop and sends new info
// to the channel. But be called in a goroutine.
func (c *DockerCollector) Stream() error {
	// TODO: refactor with collector.listeners.DockerListener
	// filters only match start/stop container events
	filters := filters.NewArgs()
	filters.Add("type", "container")
	filters.Add("event", "start")
	filters.Add("event", "die")
	eventOptions := types.EventsOptions{
		Since:   fmt.Sprintf("%d", time.Now().Unix()),
		Filters: filters,
	}

	messages, errs := c.client.Events(context.Background(), eventOptions)

	for {
		select {
		case <-c.stop:
			return nil
		case msg := <-messages:
			c.processEvent(msg)
		case err := <-errs:
			if err != nil && err != io.EOF {
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
func (c *DockerCollector) Fetch(container string) ([]string, []string, error) {
	cid := strings.TrimPrefix(container, docker.DockerEntityPrefix)
	if cid == container {
		return nil, nil, fmt.Errorf("name is not a docker container: %s", container)
	}
	return c.fetchForDockerID(cid)
}

func (c *DockerCollector) processEvent(e events.Message) {
	cID := e.Actor.ID
	out := make([]*TagInfo, 1)

	switch e.Action {
	case "die":
		out[0] = &TagInfo{Entity: docker.ContainerIDToEntityName(cID), Source: dockerCollectorName, DeleteEntity: true}
	case "start":
		low, high, _ := c.fetchForDockerID(cID)
		out[0] = &TagInfo{Entity: docker.ContainerIDToEntityName(cID), Source: dockerCollectorName, LowCardTags: low, HighCardTags: high}
	}
	c.infoOut <- out
}

func (c *DockerCollector) fetchForDockerID(cID string) ([]string, []string, error) {
	co, err := c.client.ContainerInspect(context.Background(), string(cID))
	if err != nil {
		log.Errorf("Failed to inspect container %s - %s", cID[:12], err)
		return nil, nil, err
	}
	return c.extractFromInspect(co)
}

func dockerFactory() Collector {
	return &DockerCollector{}
}

func init() {
	registerCollector(dockerCollectorName, dockerFactory)
}
