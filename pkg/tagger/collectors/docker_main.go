// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package collectors

import (
	"io"
	"strings"

	log "github.com/cihub/seelog"

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

	// viper lower-cases map keys, so extractor must lowercase before matching
	c.labelsAsTags = config.Datadog.GetStringMapString("docker_labels_as_tags")
	c.envAsTags = config.Datadog.GetStringMapString("docker_env_as_tags")

	// TODO: list and inspect existing containers once docker utils are merged

	return StreamCollection, nil
}

// Stream runs the continuous event watching loop and sends new info
// to the channel. But be called in a goroutine.
func (c *DockerCollector) Stream() error {
	messages, errs, err := c.dockerUtil.SubscribeToContainerEvents("DockerCollector")
	if err != nil {
		return err
	}

	for {
		select {
		case <-c.stop:
			return c.dockerUtil.UnsubscribeFromContainerEvents("DockerCollector")
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
func (c *DockerCollector) Fetch(container string) ([]string, []string, error) {
	cid := strings.TrimPrefix(container, docker.DockerEntityPrefix)
	if cid == container || len(cid) == 0 {
		return nil, nil, ErrNotFound
	}
	return c.fetchForDockerID(cid)
}

func (c *DockerCollector) processEvent(e *docker.ContainerEvent) {
	out := make([]*TagInfo, 1)

	switch e.Action {
	case "die":
		out[0] = &TagInfo{Entity: e.ContainerEntityName(), Source: dockerCollectorName, DeleteEntity: true}
	case "start":
		low, high, _ := c.fetchForDockerID(e.ContainerID)
		out[0] = &TagInfo{Entity: e.ContainerEntityName(), Source: dockerCollectorName, LowCardTags: low, HighCardTags: high}
	}
	c.infoOut <- out
}

func (c *DockerCollector) fetchForDockerID(cID string) ([]string, []string, error) {
	co, err := c.dockerUtil.Inspect(cID, false)
	if err != nil {
		// TODO separate "not found" and inspect error
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
