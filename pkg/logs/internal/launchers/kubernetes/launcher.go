// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet
// +build kubelet

package kubernetes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util/containersorpods"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	sourcesPkg "github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	basePath      = "/var/log/pods"
	anyLogFile    = "*.log"
	anyV19LogFile = "%s_*.log"
)

var errCollectAllDisabled = fmt.Errorf("%s disabled", config.ContainerCollectAll)

// Launcher looks for new and deleted pods to create or delete one logs-source per container.
type Launcher struct {
	sources            *sourcesPkg.LogSources
	services           *service.Services
	cop                containersorpods.Chooser
	sourcesByContainer map[string]*sourcesPkg.LogSource
	stopped            chan struct{}
	collectAll         bool
	serviceNameFunc    func(string, string) string // serviceNameFunc gets the service name from the tagger, it is in a separate field for testing purpose
	workloadmetaStore  workloadmeta.Store

	// ctx is the context for the running goroutine, set in Start
	ctx context.Context

	// cancel cancels the running goroutine
	cancel context.CancelFunc
}

// NewLauncher returns a new launcher.
func NewLauncher(sources *sourcesPkg.LogSources, services *service.Services, cop containersorpods.Chooser, collectAll bool) *Launcher {
	launcher := &Launcher{
		sources:            sources,
		services:           services,
		cop:                cop,
		sourcesByContainer: make(map[string]*sourcesPkg.LogSource),
		stopped:            make(chan struct{}),
		collectAll:         collectAll,
		serviceNameFunc:    util.ServiceNameFromTags,
		workloadmetaStore:  workloadmeta.GetGlobalStore(),
	}
	return launcher
}

// Start starts the launcher
func (l *Launcher) Start(sourceProvider launchers.SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry) {
	// only start this launcher once it's determined that we should be logging containers, and not pods.
	l.ctx, l.cancel = context.WithCancel(context.Background())
	go l.run(sourceProvider, pipelineProvider, registry)
}

// Stop stops the launcher
func (l *Launcher) Stop() {
	if l.cancel != nil {
		l.cancel()
	}

	// only stop this launcher once it's determined that we should be logging
	// pods, and not containers, but do not block trying to find out.
	if l.cop.Get() == containersorpods.LogPods {
		l.stopped <- struct{}{}
	}
}

// run handles new and deleted pods,
// the kubernetes launcher consumes new and deleted services pushed by the autodiscovery
func (l *Launcher) run(sourceProvider launchers.SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry) {
	// if we're not logging pods, then there's nothing to do
	if l.cop.Wait(l.ctx) != containersorpods.LogPods {
		return
	}

	log.Info("Starting Kubernetes launcher")
	addedServices := l.services.GetAllAddedServices()
	removedServices := l.services.GetAllRemovedServices()

	for {
		select {
		case service := <-addedServices:
			l.addSource(service)
		case service := <-removedServices:
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

	source, err := l.getSource(svc)
	if err != nil {
		if err != errCollectAllDisabled {
			log.Warnf("Invalid configuration for service %q: %v", svc.GetEntityID(), err)
		}
		return
	}

	switch svc.Type {
	case config.DockerType:
		source.SetSourceType(sourcesPkg.DockerSourceType)
	default:
		source.SetSourceType(sourcesPkg.KubernetesSourceType)
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

func (l *Launcher) getSource(svc *service.Service) (*sourcesPkg.LogSource, error) {
	containerID := svc.Identifier

	pod, err := l.workloadmetaStore.GetKubernetesPodForContainer(containerID)
	if err != nil {
		return nil, fmt.Errorf("cannot find pod for container %q: %w", containerID, err)
	}

	var container *workloadmeta.OrchestratorContainer
	for _, pc := range pod.Containers {
		if pc.ID == containerID {
			container = &pc
			break
		}
	}

	if container == nil {
		return nil, fmt.Errorf("cannot find container %q in pod %q", containerID, pod.Name)
	}

	runtimeContainer, err := l.workloadmetaStore.GetContainer(containerID)
	if err != nil {
		return nil, fmt.Errorf("cannot find container %q: %w", containerID, err)
	}
	var cfg *config.LogsConfig

	if annotation := l.getAnnotation(container.Name, pod.Annotations); annotation != "" {
		configs, err := config.ParseJSON([]byte(annotation))
		if err != nil || len(configs) == 0 {
			return nil, fmt.Errorf("could not parse kubernetes annotation %v", annotation)
		}

		// We may have more than one log configuration in the annotation, ignore those
		// unrelated to containers
		containerType := string(runtimeContainer.Runtime)
		for _, c := range configs {
			if c.Type == "" || c.Type == containerType {
				cfg = c
				break
			}
		}

		if cfg == nil {
			log.Debugf("annotation found: %v, for pod %v, container %v, but no config was usable for container log collection", annotation, pod.Name, container.Name)
		}
	}

	standardService := l.serviceNameFunc(container.Name, containers.BuildTaggerEntityName(containerID))

	if cfg == nil {
		if !l.collectAll {
			return nil, errCollectAllDisabled
		}
		// The logs source is the short image name
		logsSource := ""
		shortImageName := container.Image.ShortName
		if shortImageName == "" {
			log.Debugf("Couldn't get short image for container %q: empty ShortName", container.Name)
			// Fallback and use `kubernetes` as source name
			logsSource = kubernetesIntegration
		} else {
			logsSource = shortImageName
		}

		if standardService != "" {
			cfg = &config.LogsConfig{
				Source:  logsSource,
				Service: standardService,
			}
		} else {
			cfg = &config.LogsConfig{
				Source:  logsSource,
				Service: logsSource,
			}
		}
	}

	if cfg.Service == "" && standardService != "" {
		cfg.Service = standardService
	}

	cfg.Type = config.FileType
	cfg.Path = l.getPath(basePath, pod, container.Name)
	cfg.Identifier = container.ID
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid kubernetes annotation: %v", err)
	}

	sourceName := fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, container.Name)

	return sourcesPkg.NewLogSource(sourceName, cfg), nil
}

// configPath refers to the configuration that can be passed over a pod annotation,
// this feature is commonly named 'ad' or 'autodiscovery'.
// The pod annotation must respect the format: ad.datadoghq.com/<container_name>.logs: '[{...}]'.
const (
	configPathPrefix = "ad.datadoghq.com"
	configPathSuffix = "logs"
)

// getConfigPath returns the path of the logs-config annotation for container.
func (l *Launcher) getConfigPath(containerName string) string {
	return fmt.Sprintf("%s/%s.%s", configPathPrefix, containerName, configPathSuffix)
}

// getAnnotation returns the logs-config annotation for container if present.
// FIXME: Reuse the annotation logic from AD
func (l *Launcher) getAnnotation(containerName string, annotations map[string]string) string {
	configPath := l.getConfigPath(containerName)
	if annotation, exists := annotations[configPath]; exists {
		return annotation
	}
	return ""
}

// getPath returns a wildcard matching with any logs file of container in pod.
func (l *Launcher) getPath(basePath string, pod *workloadmeta.KubernetesPod, containerName string) string {
	// the pattern for container logs is different depending on the version of Kubernetes
	// so we need to try three possbile formats
	// until v1.9 it was `/var/log/pods/{pod_uid}/{container_name_n}.log`,
	// v.1.10 to v1.13 it was `/var/log/pods/{pod_uid}/{container_name}/{n}.log`,
	// since v1.14 it is `/var/log/pods/{pod_namespace}_{pod_name}_{pod_uid}/{container_name}/{n}.log`.
	// see: https://github.com/kubernetes/kubernetes/pull/74441 for more information.
	oldDirectory := filepath.Join(basePath, l.getPodDirectoryUntil1_13(pod))
	if _, err := os.Stat(oldDirectory); err == nil {
		v110Dir := filepath.Join(oldDirectory, containerName)
		_, err := os.Stat(v110Dir)
		if err == nil {
			log.Debugf("Logs path found for container %s, v1.13 >= kubernetes version >= v1.10", containerName)
			return filepath.Join(v110Dir, anyLogFile)
		}
		if !os.IsNotExist(err) {
			log.Debugf("Cannot get file info for %s: %v", v110Dir, err)
		}

		v19Files := filepath.Join(oldDirectory, fmt.Sprintf(anyV19LogFile, containerName))
		files, err := filepath.Glob(v19Files)
		if err == nil && len(files) > 0 {
			log.Debugf("Logs path found for container %s, kubernetes version <= v1.9", containerName)
			return v19Files
		}
		if err != nil {
			log.Debugf("Cannot get file info for %s: %v", v19Files, err)
		}
		if len(files) == 0 {
			log.Debugf("Files matching %s not found", v19Files)
		}
	}

	log.Debugf("Using the latest kubernetes logs path for container %s", containerName)
	return filepath.Join(basePath, l.getPodDirectorySince1_14(pod), containerName, anyLogFile)
}

// getPodDirectoryUntil1_13 returns the name of the directory of pod containers until Kubernetes v1.13.
func (l *Launcher) getPodDirectoryUntil1_13(pod *workloadmeta.KubernetesPod) string {
	return pod.ID
}

// getPodDirectorySince1_14 returns the name of the directory of pod containers since Kubernetes v1.14.
func (l *Launcher) getPodDirectorySince1_14(pod *workloadmeta.KubernetesPod) string {
	return fmt.Sprintf("%s_%s_%s", pod.Namespace, pod.Name, pod.ID)
}
