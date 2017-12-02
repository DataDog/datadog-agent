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
const dockerAPIVersion = "1.25"

// A Scanner listens for stdout and stderr of containers
type Scanner struct {
	pp      *pipeline.Provider
	sources []*config.IntegrationConfigLogSource
	tailers map[string]*DockerTailer
	cli     *client.Client
	auditor *auditor.Auditor
}

// New returns an initialized Scanner
func New(sources []*config.IntegrationConfigLogSource, pp *pipeline.Provider, a *auditor.Auditor) *Scanner {

	containerSources := []*config.IntegrationConfigLogSource{}
	for _, source := range sources {
		switch source.Type {
		case config.DockerType:
			containerSources = append(containerSources, source)
		default:
		}
	}

	return &Scanner{
		pp:      pp,
		sources: containerSources,
		tailers: make(map[string]*DockerTailer),
		auditor: a,
	}
}

// Start starts the Scanner
func (s *Scanner) Start() {
	err := c.setup()
	if err == nil {
		go c.run()
	}
}

// run lets the Scanner tail docker stdouts
func (s *Scanner) run() {
	ticker := time.NewTicker(scanPeriod)
	for _ = range ticker.C {
		c.scan(true)
	}
}

// scan checks for new containers we're expected to
// tail, as well as stopped containers or containers that
// restarted
func (s *Scanner) scan(tailFromBegining bool) {
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
	for containerID, tailer := range c.tailers {
		_, shouldMonitor := containersToMonitor[containerID]
		if !shouldMonitor {
			c.stopTailer(tailer)
		}
	}
}

func (s *Scanner) stopTailer(tailer *DockerTailer) {
	log.Println("Stop tailing container", c.humanReadableContainerID(tailer.ContainerID))
	tailer.Stop()
	delete(c.tailers, tailer.ContainerID)
}

func (s *Scanner) listContainers() []types.Container {
	containers, err := c.cli.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		log.Println("Can't tail containers,", err)
		log.Println("Is datadog-agent part of docker user group?")
		return []types.Container{}
	}
	return containers
}

func (s *Scanner) sourceShouldMonitorContainer(source *config.IntegrationConfigLogSource, container types.Container) bool {
	if source.Image != "" && container.Image != source.Image {
		return false
	}
	if source.Label != "" {
		_, ok := container.Labels[source.Label]
		return ok
	}
	return true
}

// Start starts the Scanner
func (s *Scanner) setup() error {
	if len(c.sources) == 0 {
		return fmt.Errorf("No container source defined")
	}

	// List available containers

	cli, err := client.NewEnvClient()
	// Docker's api updates quickly and is pretty unstable, best pinpoint it
	cli.UpdateClientVersion(dockerAPIVersion)
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
func (s *Scanner) setupTailer(cli *client.Client, container types.Container, source *config.IntegrationConfigLogSource, tailFromBegining bool, outputChan chan message.Message) {
	log.Println("Detected container", container.Image, "-", c.humanReadableContainerID(container.ID))
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

// Stop stops the Scanner and its tailers
func (s *Scanner) Stop() {
	for _, t := range c.tailers {
		t.Stop()
	}
}

func (s *Scanner) humanReadableContainerID(containerID string) string {
	return containerID[:12]
}
