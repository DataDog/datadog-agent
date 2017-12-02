// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package container

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/DataDog/datadog-agent/pkg/tagger"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/docker/docker/api/types"
	"github.com/moby/moby/client"
)

const scanPeriod = 10 * time.Second
const DOCKER_API_VERSION = "1.25"

// A ContainerInput listens for stdout and stderr of containers
type ContainerInput struct {
	pp      *pipeline.PipelineProvider
	sources []*config.IntegrationConfigLogSource
	tailers map[string]*DockerTailer
	cli     *client.Client
	auditor *auditor.Auditor
}

// New returns an initialized ContainerInput
func New(sources []*config.IntegrationConfigLogSource, pp *pipeline.PipelineProvider, a *auditor.Auditor) *ContainerInput {

	containerSources := []*config.IntegrationConfigLogSource{}
	for _, source := range sources {
		switch source.Type {
		case config.DOCKER_TYPE:
			containerSources = append(containerSources, source)
		default:
		}
	}

	return &ContainerInput{
		pp:      pp,
		sources: containerSources,
		tailers: make(map[string]*DockerTailer),
		auditor: a,
	}
}

// Start starts the ContainerInput
func (c *ContainerInput) Start() {
	err := c.setup()
	if err == nil {
		go c.run()
	}
}

// run lets the ContainerInput tail docker stdouts
func (c *ContainerInput) run() {
	ticker := time.NewTicker(scanPeriod)
	for _ = range ticker.C {
		c.scan(true)
	}
}

// scan checks for new containers we're expected to
// tail, as well as stopped containers or containers that
// restarted
func (c *ContainerInput) scan(tailFromBegining bool) {
	runningContainers := c.listContainers()
	containersToMonitor := make(map[string]bool)

	// monitor new containers, and restart tailers if needed
	for _, container := range runningContainers {
		for _, source := range c.sources {
			if c.sourceShouldMonitorContainer(source, container) {
				containersToMonitor[container.ID] = true

				tailer, isTailed := c.tailers[container.ID]
				if isTailed && tailer.shouldStop {
					c.stopTailer(tailer)
					isTailed = false
				}
				if !isTailed {
					c.setupTailer(c.cli, container, source, tailFromBegining, c.pp.NextPipelineChan())
				}
			}
		}
	}

	// stop old containers
	for containerId, tailer := range c.tailers {
		_, shouldMonitor := containersToMonitor[containerId]
		if !shouldMonitor {
			c.stopTailer(tailer)
		}
	}
}

func (c *ContainerInput) stopTailer(tailer *DockerTailer) {
	log.Println("Stop tailing container", c.HumanReadableContainerId(tailer.containerId))
	tailer.Stop()
	delete(c.tailers, tailer.containerId)
}

func (c *ContainerInput) listContainers() []types.Container {
	containers, err := c.cli.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		log.Println("Can't tail containers,", err)
		log.Println("Is datadog-agent part of docker user group?")
		return []types.Container{}
	}
	return containers
}

func (c *ContainerInput) sourceShouldMonitorContainer(source *config.IntegrationConfigLogSource, container types.Container) bool {
	if source.Image != "" && container.Image != source.Image {
		return false
	}
	if source.Label != "" {
		_, ok := container.Labels[source.Label]
		return ok
	}
	return true
}

// Start starts the ContainerInput
func (c *ContainerInput) setup() error {
	if len(c.sources) == 0 {
		return fmt.Errorf("No container source defined")
	}

	// List available containers

	cli, err := client.NewEnvClient()
	// Docker's api updates quickly and is pretty unstable, best pinpoint it
	cli.UpdateClientVersion(DOCKER_API_VERSION)
	c.cli = cli
	if err != nil {
		log.Println("Can't tail containers,", err)
		return fmt.Errorf("Can't initialize client")
	}

	// Initialize docker utils
	err = tagger.Init()
	if err != nil {
		log.Println(err)
	}

	// Start tailing monitored containers
	c.scan(false)
	return nil
}

// setupTailer sets one tailer, making it tail from the begining or the end
func (c *ContainerInput) setupTailer(cli *client.Client, container types.Container, source *config.IntegrationConfigLogSource, tailFromBegining bool, outputChan chan message.Message) {
	log.Println("Detected container", container.Image, "-", c.HumanReadableContainerId(container.ID))
	t := NewDockerTailer(cli, container, source, outputChan)
	var err error
	if tailFromBegining {
		err = t.tailFromBegining()
	} else {
		err = t.recoverTailing(c.auditor)
	}
	if err != nil {
		log.Println(err)
	}
	c.tailers[container.ID] = t
}

// Stop stops the ContainerInput and its tailers
func (c *ContainerInput) Stop() {
	for _, t := range c.tailers {
		t.Stop()
	}
}

func (c *ContainerInput) HumanReadableContainerId(containerId string) string {
	return containerId[:12]
}
