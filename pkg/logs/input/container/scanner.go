// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package container

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/versions"
	"github.com/docker/docker/client"
)

const scanPeriod = 10 * time.Second

// Supported versions of the Docker API
const (
	minVersion = "1.18"
	maxVersion = "1.25"
)

// A Scanner listens for stdout and stderr of containers
type Scanner struct {
	pp      pipeline.Provider
	sources []*config.LogSource
	tailers map[string]*DockerTailer
	cli     *client.Client
	auditor *auditor.Auditor
}

// New returns an initialized Scanner
func New(sources []*config.LogSource, pp pipeline.Provider, a *auditor.Auditor) *Scanner {

	containerSources := []*config.LogSource{}
	for _, source := range sources {
		switch source.Config.Type {
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
	err := s.setup()
	if err != nil {
		s.reportErrorToAllSources(err)
		return
	}
	go s.run()
}

// reportErrorToAllSources changes the status of all sources to Error with err
func (s *Scanner) reportErrorToAllSources(err error) {
	for _, source := range s.sources {
		source.Status.Error(err)
	}
}

// run lets the Scanner tail docker stdouts
func (s *Scanner) run() {
	ticker := time.NewTicker(scanPeriod)
	for range ticker.C {
		s.scan(true)
	}
}

// scan checks for new containers we're expected to
// tail, as well as stopped containers or containers that
// restarted
func (s *Scanner) scan(tailFromBeginning bool) {
	runningContainers := s.listContainers()
	containersToMonitor := make(map[string]bool)

	// monitor new containers, and restart tailers if needed
	for _, container := range runningContainers {
		for _, source := range s.sources {
			if s.sourceShouldMonitorContainer(source, container) {
				containersToMonitor[container.ID] = true

				tailer, isTailed := s.tailers[container.ID]
				if isTailed && tailer.shouldStop {
					s.stopTailer(tailer)
					isTailed = false
				}
				if !isTailed {
					s.setupTailer(s.cli, container, source, tailFromBeginning, s.pp.NextPipelineChan())
				}
			}
		}
	}

	// stop old containers
	for containerID, tailer := range s.tailers {
		_, shouldMonitor := containersToMonitor[containerID]
		if !shouldMonitor {
			s.stopTailer(tailer)
		}
	}
}

func (s *Scanner) stopTailer(tailer *DockerTailer) {
	log.Info("Stop tailing container ", s.humanReadableContainerID(tailer.ContainerID))
	tailer.Stop()
	delete(s.tailers, tailer.ContainerID)
}

func (s *Scanner) listContainers() []types.Container {
	containers, err := s.cli.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		log.Error("Can't tail containers, ", err)
		log.Error("Is datadog-agent part of docker user group?")
		s.reportErrorToAllSources(err)
		return []types.Container{}
	}
	return containers
}

// sourceShouldMonitorContainer returns whether a container matches a log source configuration.
// Both image and label may be used:
// - If the source defines an image, the container must match it exactly.
// - If the source defines one or several labels, at least one of them must match the labels of the container.
func (s *Scanner) sourceShouldMonitorContainer(source *config.LogSource, container types.Container) bool {
	if source.Config.Image != "" && container.Image != source.Config.Image {
		return false
	}
	if source.Config.Label != "" {
		// Expect a comma-separated list of labels, eg: foo:bar, baz
		for _, value := range strings.Split(source.Config.Label, ",") {
			// Trim whitespace, then check whether the label format is either key:value or key=value
			label := strings.TrimSpace(value)
			parts := strings.FieldsFunc(label, func(c rune) bool {
				return c == ':' || c == '='
			})
			// If we have exactly two parts, check there is a container label that matches both.
			// Otherwise fall back to checking the whole label exists as a key.
			if _, exists := container.Labels[label]; exists || len(parts) == 2 && container.Labels[parts[0]] == parts[1] {
				return true
			}
		}
		return false
	}
	return true
}

// Start starts the Scanner
func (s *Scanner) setup() error {
	if len(s.sources) == 0 {
		return fmt.Errorf("No container source defined")
	}

	cli, err := s.newDockerClient()
	if err != nil {
		log.Error("Can't tail containers, ", err)
		return fmt.Errorf("Can't initialize client")
	}
	s.cli = cli

	// Initialize docker utils
	err = tagger.Init()
	if err != nil {
		log.Warn(err)
	}

	// Start tailing monitored containers
	s.scan(false)
	return nil
}

// newDockerClient returns a new Docker client with the right API version
func (s *Scanner) newDockerClient() (*client.Client, error) {
	client, err := client.NewEnvClient()
	if err != nil {
		return nil, err
	}
	v, err := client.ServerVersion(context.Background())
	if err != nil {
		return nil, err
	}
	apiVersion, err := s.computeClientAPIVersion(v.APIVersion)
	if err != nil {
		return nil, err
	}
	client.UpdateClientVersion(apiVersion)
	return client, nil
}

// computeAPIVersion returns the version of the API that the docker client should use to be able to communicate with server
func (s *Scanner) computeClientAPIVersion(apiVersion string) (string, error) {
	if versions.LessThan(apiVersion, minVersion) {
		return "", fmt.Errorf("Docker API versions prior to %s are not supported by logs-agent, the current version is %s", minVersion, apiVersion)
	}
	if versions.LessThan(apiVersion, maxVersion) {
		return apiVersion, nil
	}
	return maxVersion, nil
}

// setupTailer sets one tailer, making it tail from the beginning or the end
func (s *Scanner) setupTailer(cli *client.Client, container types.Container, source *config.LogSource, tailFromBeginning bool, outputChan chan message.Message) {
	log.Info("Detected container ", container.Image, " - ", s.humanReadableContainerID(container.ID))
	t := NewDockerTailer(cli, container, source, outputChan)
	var err error
	if tailFromBeginning {
		err = t.tailFromBeginning()
	} else {
		err = t.recoverTailing(s.auditor)
	}
	if err != nil {
		log.Warn(err)
	}
	s.tailers[container.ID] = t
}

// Stop stops the Scanner and its tailers
func (s *Scanner) Stop() {
	for _, t := range s.tailers {
		t.Stop()
	}
}

func (s *Scanner) humanReadableContainerID(containerID string) string {
	return containerID[:12]
}
