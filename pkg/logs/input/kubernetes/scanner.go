// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package container

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

const (

	// The path to the pods log directory.
	podsDirectoryPath = "/var/log/pods"
)

// Scanner looks for new and deleted pods to start or stop one file tailer per container.
type Scanner struct {
	watcher            Watcher
	sources            *config.LogSources
	sourcesByContainer map[string]*config.LogSource
	stopped            chan struct{}
}

func Scanner(sources *config.LogSources, pp pipeline.Provider, auditor *auditor.Auditor) (*Scanner, error) {
	// initialize a pods watcher to handle added and removed pods.
	watcher, err := NewWatcher(Inotify) // TODO: drive the strategy by a configuration parameter.
	if err != nil {
		return nil, err
	}
	// initialize the tagger to collect container tags
	err = tagger.Init()
	if err != nil {
		return nil, err
	}
	return &Scanner{
		watcher:            watcher,
		sources:            sources,
		sourcesByContainer: make(map[string]*config.LogSource),
		stopped:            make(chan struct{}),
	}, nil
}

// Start starts the scanner
func (s *Scanner) Start() {
	log.Info("Starting Kubernetes scanner")
	go s.run()
	s.watcher.Start()
}

// Stop stops the scanner
func (s *Scanner) Stop() {
	log.Info("Stopping Kubernetes scanner")
	s.watcher.Stop()
	s.stopped <- struct{}{}
}

// run handles new and removed pods
func (s *Scanner) run() {
	for {
		select {
		case added := <-s.watcher.Added():
			log.Infof("adding pod: %v", pod.Metadata.Name)
			s.addSources(pod)
		case removed := <-s.watcher.Removed():
			log.Infof("removing pod %v", pod.Metadata.Name)
			s.removeSources(pod)
		case <-s.stopped:
			return
		}
	}
}

// addSources creates a new log source for each container of a new pod.
func (s *Scanner) addSources(pod *kubelet.Pod) {
	for _, container := range pod.Status.Containers {
		containerID := container.ID
		if _, exists := s.sourcesByContainer[containerID]; exists {
			continue
		}
		source := s.getSource(pod, container)
		s.sourcesByContainer[containerID] = source
		s.sources.AddSource(source)
	}
}

// removeSources removes all log sources for all the containers in pod.
func (s *Scanner) removeSources(pod *kubelet.Pod) {
	for _, container := range pod.Status.Containers {
		containerID := container.ID
		if source, exists := s.sourcesByContainer[containerID]; exists {
			delete(s.sourcesByContainer, containerID)
			s.sources.RemoveSource(source)
		}
	}
}

// kubernetesIntegration represents the name of the integration
const kubernetesIntegration = "kubernetes"

// getSource returns a new source for the container in pod
func (s *Scanner) getSource(pod *kubelet.Pod, container kubelet.ContainerStatus) *config.LogSource {
	return config.NewLogSource(s.getSourceName(pod, container), &config.LogsConfig{
		Type:    config.FileType,
		Path:    s.getPath(pod, container),
		Source:  kubernetesIntegration,
		Service: kubernetesIntegration,
		Tags:    s.getTags(container),
	})
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
