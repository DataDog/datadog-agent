// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package container

import (
	"time"

	"fmt"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/input/file"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const podScanPeriod = 1 * time.Second
const podExpiration = 5 * time.Second
const tailerSleepPeriod = 1 * time.Second
const tailerMaxOpenFiles = 2147483647

type KubeScanner struct {
	scanner            *file.Scanner
	watcher            *kubelet.PodWatcher
	sources            *config.LogSources
	sourcesByContainer map[string]*config.LogSource
	stopped            chan struct{}
}

func NewKubeScanner(sources *config.LogSources, pp pipeline.Provider, auditor *auditor.Auditor) (*KubeScanner, error) {
	watcher, err := kubelet.NewPodWatcher(podExpiration)
	if err != nil {
		return nil, err
	}
	err = tagger.Init()
	if err != nil {
		log.Warn(err)
	}
	scanner := file.New(sources, tailerMaxOpenFiles, pp, auditor, tailerSleepPeriod)
	return &KubeScanner{
		scanner:            scanner,
		watcher:            watcher,
		sources:            sources,
		sourcesByContainer: make(map[string]*config.LogSource),
		stopped:            make(chan struct{}),
	}, nil
}

// Start
func (s *KubeScanner) Start() {
	log.Info("Starting Kubernetes scanner")
	s.scanner.Start()
	go s.run()
}

// Stop
func (s *KubeScanner) Stop() {
	log.Info("Stopping Kubernetes scanner")
	s.scanner.Stop()
	s.stopped <- struct{}{}
}

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

func (s *KubeScanner) scan() {
	s.updatePods()
	s.expirePods()
}

func (s *KubeScanner) updatePods() {
	pods, err := s.watcher.PullChanges()
	if err != nil {
		log.Error("can't list changed pods", err)
		return
	}
	for _, pod := range pods {
		if pod.Status.Phase == "Running" {
			log.Infof("added pod: %v", pod.Metadata.Name)
			s.addNewSources(pod)
		}
	}
}

func (s *KubeScanner) expirePods() {
	entityIDs, err := s.watcher.Expire()
	if err != nil {
		log.Error("can't list expired pods", err)
		return
	}
	for _, entityID := range entityIDs {
		log.Infof("removed pod %v", entityID)
		pod, err := s.watcher.GetPodForEntityID(entityID)
		if err != nil {
			log.Errorf("can't find pod %v: %v", entityID, err)
			continue
		}
		for _, container := range pod.Status.Containers {
			containerID := container.ID
			if source, exists := s.sourcesByContainer[containerID]; exists {
				delete(s.sourcesByContainer, containerID)
				s.sources.RemoveSource(source)
			}
		}
	}
}

func (s *KubeScanner) addNewSources(pod *kubelet.Pod) {
	for _, container := range pod.Status.Containers {
		containerID := container.ID
		if _, exists := s.sourcesByContainer[containerID]; exists {
			continue
		}
		tags, _ := tagger.Tag(containerID, true)
		containerName := container.Name
		sourceName := pod.Metadata.Name + "/" + containerName
		source := config.NewLogSource(sourceName, &config.LogsConfig{
			Type: config.FileType,
			Path: fmt.Sprintf("/var/log/pods/%s/%s/*.log", pod.Metadata.UID, containerName),
			Tags: tags,
		})
		s.sourcesByContainer[containerID] = source
		s.sources.AddSource(source)
	}
}
