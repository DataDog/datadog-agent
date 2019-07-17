// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubelet

package kubernetes

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
)

const (
	basePath   = "/var/log/pods"
	anyLogFile = "*.log"
)

var collectAllDisabledError = fmt.Errorf("%s disabled", config.ContainerCollectAll)

// Launcher looks for new and deleted pods to create or delete one logs-source per container.
type Launcher struct {
	sources            *config.LogSources
	sourcesByContainer map[string]*config.LogSource
	stopped            chan struct{}
	kubeutil           *kubelet.KubeUtil
	addedServices      chan *service.Service
	removedServices    chan *service.Service
	collectAll         bool
}

// NewLauncher returns a new launcher.
func NewLauncher(sources *config.LogSources, services *service.Services, collectAll bool) (*Launcher, error) {
	if !isIntegrationAvailable() {
		return nil, fmt.Errorf("%s not found", basePath)
	}
	kubeutil, err := kubelet.GetKubeUtil()
	if err != nil {
		return nil, err
	}
	launcher := &Launcher{
		sources:            sources,
		sourcesByContainer: make(map[string]*config.LogSource),
		stopped:            make(chan struct{}),
		kubeutil:           kubeutil,
		collectAll:         collectAll,
	}
	err = launcher.setup()
	if err != nil {
		return nil, err
	}
	launcher.addedServices = services.GetAllAddedServices()
	launcher.removedServices = services.GetAllRemovedServices()
	return launcher, nil
}

func isIntegrationAvailable() bool {
	if _, err := os.Stat(basePath); err != nil {
		return false
	}

	return true
}

// setup initializes the pod watcher and the tagger.
func (l *Launcher) setup() error {
	// initialize the tagger to collect container tags
	tagger.Init()
	return nil
}

// Start starts the launcher
func (l *Launcher) Start() {
	log.Info("Starting Kubernetes launcher")
	go l.run()
}

// Stop stops the launcher
func (l *Launcher) Stop() {
	log.Info("Stopping Kubernetes launcher")
	l.stopped <- struct{}{}
}

// run handles new and deleted pods,
// the kubernetes launcher consumes new and deleted services pushed by the autodiscovery
func (l *Launcher) run() {
	for {
		select {
		case service := <-l.addedServices:
			l.addSource(service)
		case service := <-l.removedServices:
			l.removeSource(service)
		case <-l.stopped:
			log.Info("Kubernetes launcher stopped")
			return
		}
	}
}

// addSource creates a new log-source from a service by resolving the
// pod linked to the entityID of the service
func (l *Launcher) addSource(svc *service.Service) {
	// If the container is already tailed, we don't do anything
	// That shoudn't happen
	if _, exists := l.sourcesByContainer[svc.GetEntityID()]; exists {
		log.Warnf("A source already exist for container %v", svc.GetEntityID())
		return
	}

	pod, err := l.kubeutil.GetPodForEntityID(svc.GetEntityID())
	if err != nil {
		log.Warnf("Could not add source for container %v: %v", svc.Identifier, err)
		return
	}
	container, err := l.kubeutil.GetStatusForContainerID(pod, svc.GetEntityID())
	if err != nil {
		log.Warn(err)
		return
	}
	source, err := l.getSource(pod, container)
	if err != nil {
		if err != collectAllDisabledError {
			log.Warnf("Invalid configuration for pod %v, container %v: %v", pod.Metadata.Name, container.Name, err)
		}
		return
	}

	switch svc.Type {
	case config.DockerType:
		source.SetSourceType(config.DockerSourceType)
	default:
		source.SetSourceType(config.KubernetesSourceType)
	}

	l.sourcesByContainer[svc.GetEntityID()] = source
	l.sources.AddSource(source)
}

// removeSource removes a new log-source from a service
func (l *Launcher) removeSource(service *service.Service) {
	containerID := service.GetEntityID()
	if source, exists := l.sourcesByContainer[containerID]; exists {
		delete(l.sourcesByContainer, containerID)
		l.sources.RemoveSource(source)
	}
}

// kubernetesIntegration represents the name of the integration.
const kubernetesIntegration = "kubernetes"

// getSource returns a new source for the container in pod.
func (l *Launcher) getSource(pod *kubelet.Pod, container kubelet.ContainerStatus) (*config.LogSource, error) {
	var cfg *config.LogsConfig
	if annotation := l.getAnnotation(pod, container); annotation != "" {
		configs, err := config.ParseJSON([]byte(annotation))
		if err != nil || len(configs) == 0 {
			return nil, fmt.Errorf("could not parse kubernetes annotation %v", annotation)
		}
		cfg = configs[0]
	} else {
		if !l.collectAll {
			return nil, collectAllDisabledError
		}
		shortImageName, err := l.getShortImageName(container)
		if err != nil {
			cfg = &config.LogsConfig{
				Source:  kubernetesIntegration,
				Service: kubernetesIntegration,
			}
		} else {
			cfg = &config.LogsConfig{
				Source:  shortImageName,
				Service: shortImageName,
			}
		}
	}
	cfg.Type = config.FileType
	cfg.Path = l.getPath(basePath, pod, container)
	taggerEntityID, err := kubelet.KubeContainerIDToTaggerEntityID(container.ID)
	if err != nil {
		log.Warnf("Could not get tagger entity ID: %v", err)
		cfg.Identifier = container.ID
	} else {
		cfg.Identifier = taggerEntityID
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid kubernetes annotation: %v", err)
	}

	return config.NewLogSource(l.getSourceName(pod, container), cfg), nil
}

// configPath refers to the configuration that can be passed over a pod annotation,
// this feature is commonly named 'ad' or 'autodiscovery'.
// The pod annotation must respect the format: ad.datadoghq.com/<container_name>.logs: '[{...}]'.
const (
	configPathPrefix = "ad.datadoghq.com"
	configPathSuffix = "logs"
)

// getConfigPath returns the path of the logs-config annotation for container.
func (l *Launcher) getConfigPath(container kubelet.ContainerStatus) string {
	return fmt.Sprintf("%s/%s.%s", configPathPrefix, container.Name, configPathSuffix)
}

// getAnnotation returns the logs-config annotation for container if present.
// FIXME: Reuse the annotation logic from AD
func (l *Launcher) getAnnotation(pod *kubelet.Pod, container kubelet.ContainerStatus) string {
	configPath := l.getConfigPath(container)
	if annotation, exists := pod.Metadata.Annotations[configPath]; exists {
		return annotation
	}
	return ""
}

// getSourceName returns the source name of the container to tail.
func (l *Launcher) getSourceName(pod *kubelet.Pod, container kubelet.ContainerStatus) string {
	return fmt.Sprintf("%s/%s/%s", pod.Metadata.Namespace, pod.Metadata.Name, container.Name)
}

// getPath returns a wildcard matching with any logs file of container in pod.
func (l *Launcher) getPath(basePath string, pod *kubelet.Pod, container kubelet.ContainerStatus) string {
	// the pattern for container logs is different depending on the version of Kubernetes
	// so we need to try both format,
	// until v1.13 it was `/var/log/pods/{pod_uid}/{container_name}/{n}.log`,
	// since v1.14 it is `/var/log/pods/{pod_namespace}_{pod_name}_{pod_uid}/{container_name}/{n}.log`.
	// see: https://github.com/kubernetes/kubernetes/pull/74441 for more information.
	oldDirectory := filepath.Join(basePath, l.getPodDirectoryUntil1_13(pod))
	if _, err := os.Stat(oldDirectory); err == nil {
		return filepath.Join(oldDirectory, container.Name, anyLogFile)
	}
	return filepath.Join(basePath, l.getPodDirectorySince1_14(pod), container.Name, anyLogFile)
}

// getPodDirectoryUntil1_13 returns the name of the directory of pod containers until Kubernetes v1.13.
func (l *Launcher) getPodDirectoryUntil1_13(pod *kubelet.Pod) string {
	return pod.Metadata.UID
}

// getPodDirectorySince1_14 returns the name of the directory of pod containers since Kubernetes v1.14.
func (l *Launcher) getPodDirectorySince1_14(pod *kubelet.Pod) string {
	return fmt.Sprintf("%s_%s_%s", pod.Metadata.Namespace, pod.Metadata.Name, pod.Metadata.UID)
}

// getShortImageName returns the short image name of a container
func (l *Launcher) getShortImageName(container kubelet.ContainerStatus) (string, error) {
	_, shortName, _, err := containers.SplitImageName(container.Image)
	if err != nil {
		log.Debugf("Cannot parse image name: %v", err)
	}
	return shortName, err
}
