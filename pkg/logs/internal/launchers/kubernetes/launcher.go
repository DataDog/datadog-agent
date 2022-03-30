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
	"time"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"github.com/cenkalti/backoff"
)

const (
	basePath      = "/var/log/pods"
	anyLogFile    = "*.log"
	anyV19LogFile = "%s_*.log"
)

var errCollectAllDisabled = fmt.Errorf("%s disabled", config.ContainerCollectAll)

type retryOps struct {
	service          *service.Service
	backoff          backoff.BackOff
	removalScheduled bool
}

// Launcher looks for new and deleted pods to create or delete one logs-source per container.
type Launcher struct {
	sources            *config.LogSources
	services           *service.Services
	sourcesByContainer map[string]*config.LogSource
	stopped            chan struct{}
	kubeutil           kubelet.KubeUtilInterface
	addedServices      chan *service.Service
	removedServices    chan *service.Service
	retryOperations    chan *retryOps
	collectAll         bool
	pendingRetries     map[string]*retryOps
	serviceNameFunc    func(string, string) string // serviceNameFunc gets the service name from the tagger, it is in a separate field for testing purpose
}

// IsAvailable retrues true if the launcher is available and a retrier otherwise
func IsAvailable() (bool, *retry.Retrier) {
	if !isIntegrationAvailable() {
		if coreConfig.IsFeaturePresent(coreConfig.Kubernetes) {
			log.Warnf("Kubernetes launcher is not available. Integration not available - %s not found", basePath)
		}
		return false, nil
	}
	util, retrier := kubelet.GetKubeUtilWithRetrier()
	if util != nil {
		log.Info("Kubernetes launcher is available")
		return true, nil
	}
	log.Infof("Kubernetes launcher is not available: %v", retrier.LastError())
	return false, retrier
}

// NewLauncher returns a new launcher.
func NewLauncher(sources *config.LogSources, services *service.Services, collectAll bool) *Launcher {
	kubeutil, err := kubelet.GetKubeUtil()
	if err != nil {
		log.Errorf("KubeUtil not available, failed to create launcher: %v", err)
		return nil
	}
	launcher := &Launcher{
		sources:            sources,
		services:           services,
		sourcesByContainer: make(map[string]*config.LogSource),
		stopped:            make(chan struct{}),
		kubeutil:           kubeutil,
		collectAll:         collectAll,
		pendingRetries:     make(map[string]*retryOps),
		retryOperations:    make(chan *retryOps),
		serviceNameFunc:    util.ServiceNameFromTags,
	}
	return launcher
}

func isIntegrationAvailable() bool {
	if _, err := os.Stat(basePath); err != nil {
		return false
	}

	return true
}

// Start starts the launcher
func (l *Launcher) Start(sourceProvider launchers.SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry) {
	log.Info("Starting Kubernetes launcher")
	l.addedServices = l.services.GetAllAddedServices()
	l.removedServices = l.services.GetAllRemovedServices()
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
		case ops := <-l.retryOperations:
			l.addSource(ops.service)
		case <-l.stopped:
			log.Info("Kubernetes launcher stopped")
			return
		}
	}
}

func (l *Launcher) scheduleServiceForRetry(svc *service.Service) {
	containerID := svc.GetEntityID()
	ops, exists := l.pendingRetries[containerID]
	if !exists {
		b := &backoff.ExponentialBackOff{
			InitialInterval:     500 * time.Millisecond,
			RandomizationFactor: 0,
			Multiplier:          2,
			MaxInterval:         5 * time.Second,
			MaxElapsedTime:      30 * time.Second,
			Clock:               backoff.SystemClock,
		}
		b.Reset()
		ops = &retryOps{
			service:          svc,
			backoff:          b,
			removalScheduled: false,
		}
		l.pendingRetries[containerID] = ops
	}
	l.delayRetry(ops)
}

func (l *Launcher) delayRetry(ops *retryOps) {
	delay := ops.backoff.NextBackOff()
	if delay == backoff.Stop {
		log.Warnf("Unable to add source for container %v", ops.service.GetEntityID())
		delete(l.pendingRetries, ops.service.GetEntityID())
		return
	}
	go func() {
		<-time.After(delay)
		l.retryOperations <- ops
	}()
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

	pod, err := l.kubeutil.GetPodForEntityID(context.TODO(), svc.GetEntityID())
	if err != nil {
		if errors.IsRetriable(err) {
			// Attempt to reschedule the source later
			log.Debugf("Failed to fetch pod info for container %v, will retry: %v", svc.Identifier, err)
			l.scheduleServiceForRetry(svc)
			return
		}
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
		if err != errCollectAllDisabled {
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

	// Clean-up retry logic
	if ops, exists := l.pendingRetries[svc.GetEntityID()]; exists {
		if ops.removalScheduled {
			// A removal was emitted while addSource was being retried
			l.removeSource(ops.service)
		}
		delete(l.pendingRetries, svc.GetEntityID())
	}
}

// removeSource removes a new log-source from a service
func (l *Launcher) removeSource(service *service.Service) {
	containerID := service.GetEntityID()
	if ops, exists := l.pendingRetries[containerID]; exists {
		// Service was added unsuccessfully and is being retried
		ops.removalScheduled = true
		return
	}
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
	standardService := l.serviceNameFunc(container.Name, getTaggerEntityID(container.ID))
	if annotation := l.getAnnotation(pod, container); annotation != "" {
		configs, err := config.ParseJSON([]byte(annotation))
		if err != nil || len(configs) == 0 {
			return nil, fmt.Errorf("could not parse kubernetes annotation %v", annotation)
		}
		// We may have more than one log configuration in the annotation, ignore those
		// unrelated to containers
		containerType, _ := containers.SplitEntityName(container.ID)
		for _, c := range configs {
			if c.Type == "" || c.Type == containerType {
				cfg = c
				break
			}
		}
		if cfg == nil {
			log.Debugf("annotation found: %v, for pod %v, container %v, but no config was usable for container log collection", annotation, pod.Metadata.Name, container.Name)
		}
	}

	if cfg == nil {
		if !l.collectAll {
			return nil, errCollectAllDisabled
		}
		// The logs source is the short image name
		logsSource := ""
		shortImageName, err := l.getShortImageName(pod, container.Name)
		if err != nil {
			log.Debugf("Couldn't get short image for container '%s': %v", container.Name, err)
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
	cfg.Path = l.getPath(basePath, pod, container)
	cfg.Identifier = kubelet.TrimRuntimeFromCID(container.ID)
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid kubernetes annotation: %v", err)
	}

	return config.NewLogSource(l.getSourceName(pod, container), cfg), nil
}

// getTaggerEntityID builds an entity ID from a kubernetes container ID
// Transforms the <runtime>:// prefix into container_id://
// Returns the original container ID if an error occurred
func getTaggerEntityID(ctrID string) string {
	taggerEntityID, err := kubelet.KubeContainerIDToTaggerEntityID(ctrID)
	if err != nil {
		log.Warnf("Could not get tagger entity ID: %v", err)
		return ctrID
	}
	return taggerEntityID
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
	// so we need to try three possbile formats
	// until v1.9 it was `/var/log/pods/{pod_uid}/{container_name_n}.log`,
	// v.1.10 to v1.13 it was `/var/log/pods/{pod_uid}/{container_name}/{n}.log`,
	// since v1.14 it is `/var/log/pods/{pod_namespace}_{pod_name}_{pod_uid}/{container_name}/{n}.log`.
	// see: https://github.com/kubernetes/kubernetes/pull/74441 for more information.
	oldDirectory := filepath.Join(basePath, l.getPodDirectoryUntil1_13(pod))
	if _, err := os.Stat(oldDirectory); err == nil {
		v110Dir := filepath.Join(oldDirectory, container.Name)
		_, err := os.Stat(v110Dir)
		if err == nil {
			log.Debugf("Logs path found for container %s, v1.13 >= kubernetes version >= v1.10", container.Name)
			return filepath.Join(v110Dir, anyLogFile)
		}
		if !os.IsNotExist(err) {
			log.Debugf("Cannot get file info for %s: %v", v110Dir, err)
		}

		v19Files := filepath.Join(oldDirectory, fmt.Sprintf(anyV19LogFile, container.Name))
		files, err := filepath.Glob(v19Files)
		if err == nil && len(files) > 0 {
			log.Debugf("Logs path found for container %s, kubernetes version <= v1.9", container.Name)
			return v19Files
		}
		if err != nil {
			log.Debugf("Cannot get file info for %s: %v", v19Files, err)
		}
		if len(files) == 0 {
			log.Debugf("Files matching %s not found", v19Files)
		}
	}

	log.Debugf("Using the latest kubernetes logs path for container %s", container.Name)
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
func (l *Launcher) getShortImageName(pod *kubelet.Pod, containerName string) (string, error) {
	containerSpec, err := l.kubeutil.GetSpecForContainerName(pod, containerName)
	if err != nil {
		return "", err
	}
	_, shortName, _, err := containers.SplitImageName(containerSpec.Image)
	if err != nil {
		log.Debugf("Cannot parse image name: %v", err)
	}
	return shortName, err
}
