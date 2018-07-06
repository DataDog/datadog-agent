// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package container

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/input/file"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// the scan period
	podScanPeriod = 1 * time.Second
	// the amount of time after which a pod is considered as deleted
	podExpiration = 5 * time.Second
	// the amount of time a tailer waits before reading again a file when reaching the very end
	tailerSleepPeriod = 1 * time.Second
	// the maximum number of files that can be open simultaneously
	tailerMaxOpenFiles = 1024
)

// KubeScanner looks for new and deleted pods to start or stop one file tailer per container.
type KubeScanner struct {
	fileScanner        *file.Scanner
	watcher            *kubelet.PodWatcher
	sources            *config.LogSources
	sourcesByContainer map[string]*config.LogSource
	stopped            chan struct{}
}

// NewKubeScanner returns a new scanner.
func NewKubeScanner(sources *config.LogSources, pp pipeline.Provider, auditor *auditor.Auditor) (*KubeScanner, error) {
	// initialize a pod watcher to list new and expired pods
	watcher, err := kubelet.NewPodWatcher(podExpiration)
	if err != nil {
		return nil, err
	}
	// initialize the tagger to collect container tags
	err = tagger.Init()
	if err != nil {
		return nil, err
	}
	// initialize a file scanner to collect logs from container files.
	fileScanner := file.New(sources, tailerMaxOpenFiles, pp, auditor, tailerSleepPeriod)
	return &KubeScanner{
		fileScanner:        fileScanner,
		watcher:            watcher,
		sources:            sources,
		sourcesByContainer: make(map[string]*config.LogSource),
		stopped:            make(chan struct{}),
	}, nil
}

// Start starts the scanner
func (s *KubeScanner) Start() {
	log.Info("Starting Kubernetes scanner")
	s.fileScanner.Start()
	go s.run()
}

// Stop stops the scanner
func (s *KubeScanner) Stop() {
	log.Info("Stopping Kubernetes scanner")
	s.fileScanner.Stop()
	s.stopped <- struct{}{}
}

// run runs periodically a scan to detect new and deleted pod.
func (s *KubeScanner) run() {
	ticker := time.NewTicker(podScanPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.scan()
		case <-s.stopped:
			return
		}
	}
}

// scan handles new and deleted pods.
func (s *KubeScanner) scan() {
	s.updatePods()
	s.expirePods()
}

// updatePods pulls the new pods and creates new log sources.
func (s *KubeScanner) updatePods() {
	pods, err := s.watcher.PullChanges()
	if err != nil {
		log.Error("can't list changed pods", err)
		return
	}
	for _, pod := range pods {
		if pod.Status.Phase == "Running" {
			log.Infof("adding pod: %v", pod.Metadata.Name)
			s.addNewSources(pod)
		}
	}
}

// expirePods fetches all expired pods and removes the corresponding log sources.
func (s *KubeScanner) expirePods() {
	entityIDs, err := s.watcher.Expire()
	if err != nil {
		log.Error("can't list expired pods", err)
		return
	}
	for _, entityID := range entityIDs {
		log.Infof("removing pod %v", entityID)
		pod, err := s.watcher.GetPodForEntityID(entityID)
		if err != nil {
			log.Errorf("can't find pod %v: %v", entityID, err)
			continue
		}
		s.removeSources(pod)
	}
}

// addNewSources creates a new log source for each container of a new pod.
func (s *KubeScanner) addNewSources(pod *kubelet.Pod) {
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
	sourceName := fmt.Sprintf("%s/%s/%s", pod.Metadata.Namespace, pod.Metadata.Name, container.Name)
	return config.NewLogSource(sourceName, &config.LogsConfig{
		Type:    config.FileType,
		Path:    s.getPath(pod, container),
		Source:  kubernetesIntegration,
		Service: kubernetesIntegration,
		Tags:    s.getTags(container),
	})
}

// getPath returns the path where all the logs of the container of the pod are stored.
func (s *KubeScanner) getPath(pod *kubelet.Pod, container kubelet.ContainerStatus) string {
	return fmt.Sprintf("/var/log/pods/%s/%s/*.log", pod.Metadata.UID, container.Name)
}

// getTags returns all the tags of the container
func (s *KubeScanner) getTags(container kubelet.ContainerStatus) []string {
	tags, _ := tagger.Tag(container.ID, true)
	return tags
}
