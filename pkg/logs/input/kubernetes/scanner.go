// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubernetes

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
)

// The path to the pods log directory.
const podsDirectoryPath = "/var/log/pods"

// Scanner looks for new and deleted pods to create or delete one logs-source per container.
type Scanner struct {
	podProvider        *PodProvider
	sources            *config.LogSources
	services           *service.Services
	sourcesByContainer map[string]*config.LogSource
	stopped            chan struct{}
}

// NewScanner returns a new scanner.
func NewScanner(sources *config.LogSources, services *service.Services) (*Scanner, error) {
	scanner := &Scanner{
		sources:            sources,
		services:           services,
		sourcesByContainer: make(map[string]*config.LogSource),
		stopped:            make(chan struct{}),
	}
	err := scanner.setup()
	if err != nil {
		return nil, err
	}
	return scanner, nil
}

// setup initializes the pod watcher and the tagger.
func (s *Scanner) setup() error {
	var err error
	// initialize a pod provider to retrieve added and deleted pods.
	s.podProvider, err = NewPodProvider()
	if err != nil {
		return err
	}
	// initialize the tagger to collect container tags
	err = tagger.Init()
	if err != nil {
		return err
	}
	return nil
}

// Start starts the scanner
func (s *Scanner) Start() {
	log.Info("Starting Kubernetes scanner")
	go s.run()
	s.podProvider.Start()
}

// Stop stops the scanner
func (s *Scanner) Stop() {
	log.Info("Stopping Kubernetes scanner")
	s.podProvider.Stop()
	s.stopped <- struct{}{}
}

// run handles new and deleted pods,
// the kubernetes scanner consumes new and deleted pods directly using a pod watcher
// but as the logs-agent has been plugged to autodiscovery, the scanner should use sources and services instead.
// FIXME: consume services and sources
func (s *Scanner) run() {
	for {
		select {
		case pod := <-s.podProvider.Added:
			log.Infof("Adding pod: %v", pod.Metadata.Name)
			s.addSources(pod)
		case pod := <-s.podProvider.Removed:
			log.Infof("Removing pod %v", pod.Metadata.Name)
			s.removeSources(pod)
		case <-s.stopped:
			return
		}
	}
}

// addSources creates new log-sources for each container of the pod.
func (s *Scanner) addSources(pod *kubelet.Pod) {
	for _, container := range pod.Status.Containers {
		containerID := container.ID
		if _, exists := s.sourcesByContainer[containerID]; exists {
			continue
		}
		source, err := s.getSource(pod, container)
		if err != nil {
			log.Warnf("Invalid configuration for pod %v, container %v: %v", pod.Metadata.Name, container.Name, err)
			continue
		}
		s.sourcesByContainer[containerID] = source
		s.sources.AddSource(source)
	}
}

// removeSources removes all the log-sources of all the containers of the pod.
func (s *Scanner) removeSources(pod *kubelet.Pod) {
	for _, container := range pod.Status.Containers {
		containerID := container.ID
		if source, exists := s.sourcesByContainer[containerID]; exists {
			delete(s.sourcesByContainer, containerID)
			s.sources.RemoveSource(source)
		}
	}
}

// kubernetesIntegration represents the name of the integration.
const kubernetesIntegration = "kubernetes"

// getSource returns a new source for the container in pod.
func (s *Scanner) getSource(pod *kubelet.Pod, container kubelet.ContainerStatus) (*config.LogSource, error) {
	var cfg *config.LogsConfig
	if annotation := s.getAnnotation(pod, container); annotation != "" {
		configs, err := config.ParseJSON([]byte(annotation))
		if err != nil || len(configs) == 0 {
			return nil, fmt.Errorf("could not parse kubernetes annotation %v", annotation)
		}
		cfg = configs[0]
	} else {
		cfg = &config.LogsConfig{
			Source:  kubernetesIntegration,
			Service: kubernetesIntegration,
		}
	}
	cfg.Type = config.FileType
	cfg.Path = s.getPath(pod, container)
	cfg.Identifier = container.ID
	cfg.Tags = append(cfg.Tags, s.getTags(container)...)
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid kubernetes annotation: %v", err)
	}
	if err := cfg.Compile(); err != nil {
		return nil, fmt.Errorf("could not compile kubernetes annotation: %v", err)
	}
	return config.NewLogSource(s.getSourceName(pod, container), cfg), nil
}

// configPath refers to the configuration that can be passed over a pod annotation,
// this feature is commonly named 'ad' or 'autodicovery'.
// The pod annotation must respect the format: ad.datadoghq.com/<container_name>.logs: '[{...}]'.
const (
	configPathPrefix = "ad.datadoghq.com"
	configPathSuffix = "logs"
)

// getConfigPath returns the path of the logs-config annotation for container.
func (s *Scanner) getConfigPath(container kubelet.ContainerStatus) string {
	return fmt.Sprintf("%s/%s.%s", configPathPrefix, container.Name, configPathSuffix)
}

// getAnnotation returns the logs-config annotation for container if present.
func (s *Scanner) getAnnotation(pod *kubelet.Pod, container kubelet.ContainerStatus) string {
	configPath := s.getConfigPath(container)
	if annotation, exists := pod.Metadata.Annotations[configPath]; exists {
		return annotation
	}
	return ""
}

// getSourceName returns the source name of the container to tail.
func (s *Scanner) getSourceName(pod *kubelet.Pod, container kubelet.ContainerStatus) string {
	return fmt.Sprintf("%s/%s/%s", pod.Metadata.Namespace, pod.Metadata.Name, container.Name)
}

// getPath returns the path where all the logs of the container of the pod are stored.
func (s *Scanner) getPath(pod *kubelet.Pod, container kubelet.ContainerStatus) string {
	return fmt.Sprintf("%s/%s/%s/*.log", podsDirectoryPath, pod.Metadata.UID, container.Name)
}

// getTags returns all the tags of the container
func (s *Scanner) getTags(container kubelet.ContainerStatus) []string {
	tags, _ := tagger.Tag(container.ID, true)
	return tags
}
