// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubelet

package kubernetes

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
)

// The path to the pods log directory.
const podsDirectoryPath = "/var/log/pods"

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
		return nil, fmt.Errorf("%s not found", podsDirectoryPath)
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
	if _, err := os.Stat(podsDirectoryPath); err != nil {
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
		// docker uses the NJSON format as opposed to other container runtimes
		// to write log lines to files so there is no need for the custom kubernetes parser.
		break
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
	cfg.Path = l.getPath(pod, container)
	cfg.Identifier = container.ID
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

// getPath returns the path where all the logs of the container of the pod are stored.
func (l *Launcher) getPath(pod *kubelet.Pod, container kubelet.ContainerStatus) string {
	return fmt.Sprintf("%s/%s/%s/*.log", podsDirectoryPath, pod.Metadata.UID, container.Name)
}

// getShortImageName returns the short image name of a container
func (l *Launcher) getShortImageName(container kubelet.ContainerStatus) (string, error) {
	_, shortName, _, err := containers.SplitImageName(container.Image)
	if err != nil {
		log.Debugf("Cannot parse image name: %v", err)
	}
	return shortName, err
}
