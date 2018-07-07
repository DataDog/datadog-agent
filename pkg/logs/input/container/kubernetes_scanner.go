// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package container

import (
	"fmt"
	"path/filepath"

	"github.com/fsnotify/fsnotify"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

const (

	// The path to the pods log directory.
	podsDirectoryPath = "/var/log/pods"
)

// KubeScanner looks for new and deleted pods to start or stop one file tailer per container.
type KubeScanner struct {
	watcher            *fsnotify.Watcher
	kubeUtil           *kubelet.KubeUtil
	sources            *config.LogSources
	sourcesByContainer map[string]*config.LogSource
	stopped            chan struct{}
}

// NewKubeScanner returns a new scanner.
func NewKubeScanner(sources *config.LogSources) (*KubeScanner, error) {
	// initialize a file system watcher to list added and removed pod directories.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err = watcher.Add(podsDirectoryPath); err != nil {
		return nil, err
	}
	// initialize kubeUtil to request pods from podUIDs
	kubeUtil, err := kubelet.GetKubeUtil()
	if err != nil {
		return nil, err
	}
	// initialize the tagger to collect container tags
	err = tagger.Init()
	if err != nil {
		return nil, err
	}
	return &KubeScanner{
		watcher:            watcher,
		kubeUtil:           kubeUtil,
		sources:            sources,
		sourcesByContainer: make(map[string]*config.LogSource),
		stopped:            make(chan struct{}),
	}, nil
}

// Start starts the scanner
func (s *KubeScanner) Start() {
	log.Info("Starting Kubernetes scanner")
	go s.run()
}

// Stop stops the scanner
func (s *KubeScanner) Stop() {
	log.Info("Stopping Kubernetes scanner")
	s.watcher.Close()
	s.stopped <- struct{}{}
}

// run runs periodically a scan to detect new and deleted pod.
func (s *KubeScanner) run() {
	for {
		select {
		case event := <-s.watcher.Events:
			s.handle(event)
		case err := <-s.watcher.Errors:
			log.Warnf("an error occured scanning %v: %v", podsDirectoryPath, err)
		case <-s.stopped:
			return
		}
	}
}

// handle handles new events on the file system in the '/var/log/pods' directory
// to create or remove log sources to start or stop file tailers.
func (s *KubeScanner) handle(event fsnotify.Event) {
	pod, err := s.getPod(event.Name)
	if err != nil {
		log.Error(err)
		return
	}
	switch event.Op {
	case fsnotify.Create:
		log.Infof("adding pod: %v", pod.Metadata.Name)
		s.addSources(pod)
	case fsnotify.Remove:
		log.Infof("removing pod %v", pod.Metadata.Name)
		s.removeSources(pod)
	}
}

// getPod returns the pod reversed from the log path with format '/var/log/pods/podUID'.
func (s *KubeScanner) getPod(path string) (*kubelet.Pod, error) {
	podUID := filepath.Base(path)
	pod, err := s.kubeUtil.GetPodFromUID(podUID)
	return pod, fmt.Errorf("can't find pod with id %v: %v", podUID, err)
}

// addSources creates a new log source for each container of a new pod.
func (s *KubeScanner) addSources(pod *kubelet.Pod) {
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
func (s *KubeScanner) removeSources(pod *kubelet.Pod) {
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
func (s *KubeScanner) getSource(pod *kubelet.Pod, container kubelet.ContainerStatus) *config.LogSource {
	return config.NewLogSource(s.getSourceName(pod, container), &config.LogsConfig{
		Type:    config.FileType,
		Path:    s.getPath(pod, container),
		Source:  kubernetesIntegration,
		Service: kubernetesIntegration,
		Tags:    s.getTags(container),
	})
}

// getSourceName returns the source name of the container to tail.
func (s *KubeScanner) getSourceName(pod *kubelet.Pod, container kubelet.ContainerStatus) string {
	return fmt.Sprintf("%s/%s/%s", pod.Metadata.Namespace, pod.Metadata.Name, container.Name)
}

// getPath returns the path where all the logs of the container of the pod are stored.
func (s *KubeScanner) getPath(pod *kubelet.Pod, container kubelet.ContainerStatus) string {
	return fmt.Sprintf("%s/%s/%s/*.log", podsDirectoryPath, pod.Metadata.UID, container.Name)
}

// getTags returns all the tags of the container
func (s *KubeScanner) getTags(container kubelet.ContainerStatus) []string {
	tags, _ := tagger.Tag(container.ID, true)
	return tags
}
