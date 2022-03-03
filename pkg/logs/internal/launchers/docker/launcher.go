// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package docker

import (
	"context"
	"fmt"
	"sync"
	"time"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/internal/tailers/docker"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	dockerutilpkg "github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

const (
	backoffInitialDuration = 1 * time.Second
	backoffMaxDuration     = 60 * time.Second
)

type sourceInfoPair struct {
	source *config.LogSource
	info   *config.MappedInfo
}

// A Launcher starts and stops new tailers for every new containers discovered
// by autodiscovery.
//
// Note that despite being named "Docker", this is a generic container-related
// launcher.
type Launcher struct {
	pipelineProvider   pipeline.Provider
	addedSources       chan *config.LogSource
	removedSources     chan *config.LogSource
	addedServices      chan *service.Service
	removedServices    chan *service.Service
	activeSources      []*config.LogSource
	pendingContainers  map[string]*Container
	tailers            map[string]*tailer.Tailer
	registry           auditor.Registry
	runtime            coreConfig.Feature
	stop               chan struct{}
	erroredContainerID chan string
	lock               *sync.Mutex
	collectAllSource   *config.LogSource
	collectAllInfo     *config.MappedInfo
	readTimeout        time.Duration               // client read timeout to set on the created tailer
	serviceNameFunc    func(string, string) string // serviceNameFunc gets the service name from the tagger, it is in a separate field for testing purpose

	forceTailingFromFile   bool                      // will ignore known offset and always tail from file
	tailFromFile           bool                      // If true docker will be tailed from the corresponding log file
	fileSourcesByContainer map[string]sourceInfoPair // Keep track of locally generated sources
	sources                *config.LogSources        // To schedule file source when taileing container from file
	services               *service.Services
}

// IsAvailable retrues true if the launcher is available and a retrier otherwise
func IsAvailable() (bool, *retry.Retrier) {
	if !coreConfig.IsFeaturePresent(coreConfig.Docker) {
		return false, nil
	}

	util, retrier := dockerutilpkg.GetDockerUtilWithRetrier()
	if util != nil {
		log.Info("Docker launcher is available")
		return true, nil
	}

	return false, retrier
}

// NewLauncher returns a new launcher
func NewLauncher(readTimeout time.Duration, sources *config.LogSources, services *service.Services, tailFromFile, forceTailingFromFile bool) *Launcher {
	if _, err := dockerutilpkg.GetDockerUtil(); err != nil {
		log.Errorf("DockerUtil not available, failed to create launcher: %v", err)
		return nil
	}

	var runtime coreConfig.Feature
	for _, rt := range []coreConfig.Feature{
		coreConfig.Docker,
		coreConfig.Containerd,
		coreConfig.Cri,
		coreConfig.Podman,
	} {
		if coreConfig.IsFeaturePresent(rt) {
			runtime = rt
			break
		}
	}

	launcher := &Launcher{
		tailers:                make(map[string]*tailer.Tailer),
		pendingContainers:      make(map[string]*Container),
		runtime:                runtime,
		stop:                   make(chan struct{}),
		erroredContainerID:     make(chan string),
		lock:                   &sync.Mutex{},
		readTimeout:            readTimeout,
		serviceNameFunc:        util.ServiceNameFromTags,
		sources:                sources,
		services:               services,
		forceTailingFromFile:   forceTailingFromFile,
		tailFromFile:           tailFromFile,
		fileSourcesByContainer: make(map[string]sourceInfoPair),
		collectAllInfo:         config.NewMappedInfo("Container Info"),
	}

	if tailFromFile {
		if err := launcher.checkContainerLogfileAccess(); err != nil {
			log.Errorf("Could not access container log files: %v; falling back on tailing from container runtime socket", err)
			launcher.tailFromFile = false
		}
	}

	return launcher
}

// Start starts the Launcher
func (l *Launcher) Start(sourceProvider launchers.SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry) {
	log.Info("Starting Docker launcher")
	l.pipelineProvider = pipelineProvider
	l.registry = registry
	l.addedSources = sourceProvider.GetAddedForType(config.DockerType)
	l.removedSources = sourceProvider.GetRemovedForType(config.DockerType)
	l.addedServices = l.services.GetAddedServicesForType(config.DockerType)
	l.removedServices = l.services.GetRemovedServicesForType(config.DockerType)
	go l.run()
}

// Stop stops the Launcher and its tailers in parallel,
// this call returns only when all the tailers are stopped.
func (l *Launcher) Stop() {
	log.Info("Stopping Docker launcher")
	l.stop <- struct{}{}
	stopper := startstop.NewParallelStopper()
	l.lock.Lock()
	var containerIDs []string
	for _, tailer := range l.tailers {
		stopper.Add(tailer)
		containerIDs = append(containerIDs, tailer.ContainerID)
	}
	l.lock.Unlock()
	for _, containerID := range containerIDs {
		l.removeTailer(containerID)
	}
	stopper.Stop()
}

// run starts and stops new tailers when it receives a new source
// or a new service which is mapped to a container.
func (l *Launcher) run() {
	for {
		select {
		case service := <-l.addedServices:
			// detected a new container running on the host,
			dockerutil, err := dockerutilpkg.GetDockerUtil()
			if err != nil {
				log.Warnf("Could not use docker client, logs for container %s won’t be collected: %v", service.Identifier, err)
				continue
			}
			dockerContainer, err := dockerutil.Inspect(context.TODO(), service.Identifier, false)
			if err != nil {
				log.Warnf("Could not find container with id: %v", err)
				continue
			}
			container := NewContainer(dockerContainer, service)
			source := container.FindSource(l.activeSources)
			switch {
			case source != nil:
				// a source matches with the container, start a new tailer
				l.startTailer(container, source)
			default:
				// no source matches with the container but a matching source may not have been
				// emitted yet or the container may contain an autodiscovery identifier
				// so it's put in a cache until a matching source is found.
				l.pendingContainers[service.Identifier] = container
			}
		case source := <-l.addedSources:
			// detected a new source that has been created either from a configuration file,
			// a docker label or a pod annotation.
			l.activeSources = append(l.activeSources, source)
			pendingContainers := make(map[string]*Container)
			for _, container := range l.pendingContainers {
				if container.IsMatch(source) {
					// found a container matching the new source, start a new tailer
					l.startTailer(container, source)
				} else {
					// keep the container in cache until
					pendingContainers[container.service.Identifier] = container
				}
			}
			// keep the containers that have not found any source yet for next iterations
			l.pendingContainers = pendingContainers
		case source := <-l.removedSources:
			for i, src := range l.activeSources {
				if src == source {
					// no need to stop any tailer here, it will be stopped after receiving a
					// "remove service" event.
					l.activeSources = append(l.activeSources[:i], l.activeSources[i+1:]...)
					break
				}
			}
		case service := <-l.removedServices:
			// detected that a container has been stopped.
			containerID := service.Identifier
			l.stopTailer(containerID)
			delete(l.pendingContainers, containerID)
		case containerID := <-l.erroredContainerID:
			go l.restartTailer(containerID)
		case <-l.stop:
			// no docker container should be tailed anymore
			return
		}
	}
}

// overrideSource create a new source with the image short name if the source is ContainerCollectAll
func (l *Launcher) overrideSource(container *Container, source *config.LogSource) *config.LogSource {
	standardService := l.serviceNameFunc(container.container.Name, dockerutilpkg.ContainerIDToTaggerEntityName(container.container.ID))
	if source.Name != config.ContainerCollectAll {
		if source.Config.Service == "" && standardService != "" {
			source.Config.Service = standardService
		}
		return source
	}

	if l.collectAllSource == nil {
		l.collectAllSource = source
		l.collectAllSource.RegisterInfo(l.collectAllInfo)
	}

	shortName, err := container.getShortImageName(context.TODO())
	containerID := container.service.Identifier
	if err != nil {
		log.Warnf("Could not get short image name for container %v: %v", dockerutilpkg.ShortContainerID(containerID), err)
		return source
	}

	l.collectAllInfo.SetMessage(containerID, fmt.Sprintf("Container ID: %s, Image: %s, Created: %s, Tailing from the Docker socket", dockerutilpkg.ShortContainerID(containerID), shortName, container.container.Created))

	newSource := newOverridenSource(standardService, shortName, source.Status)
	newSource.ParentSource = source
	return newSource
}

// getFileSource create a new file source with the image short name if the source is ContainerCollectAll
func (l *Launcher) getFileSource(container *Container, source *config.LogSource) sourceInfoPair {
	containerID := container.service.Identifier

	// If containerCollectAll is set - we use the global collectAllInfo, otherwise we create a new info for this source
	var sourceInfo *config.MappedInfo

	// Populate the collectAllSource if we don't have it yet
	if source.Name == config.ContainerCollectAll && l.collectAllSource == nil {
		l.collectAllSource = source
		l.collectAllSource.RegisterInfo(l.collectAllInfo)
		sourceInfo = l.collectAllInfo
	} else {
		sourceInfo = config.NewMappedInfo("Container Info")
		source.RegisterInfo(sourceInfo)
	}

	standardService := l.serviceNameFunc(container.container.Name, dockerutilpkg.ContainerIDToTaggerEntityName(containerID))
	shortName, err := container.getShortImageName(context.TODO())
	if err != nil {
		log.Warnf("Could not get short image name for container %v: %v", dockerutilpkg.ShortContainerID(containerID), err)
	}

	// Update parent source with additional information
	sourceInfo.SetMessage(containerID, fmt.Sprintf("Container ID: %s, Image: %s, Created: %s, Tailing from file: %s", dockerutilpkg.ShortContainerID(containerID), shortName, container.container.Created, l.getContainerLogfilePath(containerID)))

	// When ContainerCollectAll is not enabled, we try to derive the service and source names from container labels
	// provided by AD (in this case, the parent source config). Otherwise we use the standard service or short image
	// name for the service name and always use the short image name for the source name.
	var serviceName string
	if source.Name != config.ContainerCollectAll && source.Config.Service != "" {
		serviceName = source.Config.Service
	} else if standardService != "" {
		serviceName = standardService
	} else {
		serviceName = shortName
	}

	sourceName := shortName
	if source.Name != config.ContainerCollectAll && source.Config.Source != "" {
		sourceName = source.Config.Source
	}

	// New file source that inherit most of its parent properties
	fileSource := config.NewLogSource(source.Name, &config.LogsConfig{
		Type:             config.FileType,
		Identifier:       containerID,
		Path:             l.getContainerLogfilePath(containerID),
		Service:          serviceName,
		Source:           sourceName,
		Tags:             source.Config.Tags,
		ProcessingRules:  source.Config.ProcessingRules,
		ContainerRuntime: l.runtime,
	})
	fileSource.SetSourceType(config.DockerSourceType)
	fileSource.Status = source.Status
	fileSource.ParentSource = source
	return sourceInfoPair{source: fileSource, info: sourceInfo}
}

// newOverridenSource is separated from overrideSource for testing purpose
func newOverridenSource(standardService, shortName string, status *config.LogStatus) *config.LogSource {
	var serviceName string
	if standardService != "" {
		serviceName = standardService
	} else {
		serviceName = shortName
	}

	overridenSource := config.NewLogSource(config.ContainerCollectAll, &config.LogsConfig{
		Type:    config.DockerType,
		Service: serviceName,
		Source:  shortName,
	})
	overridenSource.Status = status
	return overridenSource
}

// startTailer starts a new tailer for the container matching with the source.
func (l *Launcher) startTailer(container *Container, source *config.LogSource) {
	if l.shouldTailFromFile(container) {
		l.scheduleFileSource(container, source)
	} else {
		l.startSocketTailer(container, source)
	}
}

func (l *Launcher) shouldTailFromFile(container *Container) bool {
	if !l.tailFromFile {
		return false
	}
	// Unsure this one is really useful, user could be instructed to clean up the registry
	if l.forceTailingFromFile {
		return true
	}
	// Check if there is a known offset for that container, if so keep tailing
	// the container from the docker socket
	registryID := fmt.Sprintf("docker:%s", container.service.Identifier)
	offset := l.registry.GetOffset(registryID)
	return offset == ""
}

func (l *Launcher) scheduleFileSource(container *Container, source *config.LogSource) {
	containerID := container.service.Identifier
	if _, isTailed := l.fileSourcesByContainer[containerID]; isTailed {
		log.Warnf("Can't tail twice the same container: %v", dockerutilpkg.ShortContainerID(containerID))
		return
	}
	// fileSource is a new source using the original source as its parent
	fileSource := l.getFileSource(container, source)
	fileSource.source.ParentSource.HideFromStatus()

	// Keep source for later unscheduling
	l.fileSourcesByContainer[containerID] = fileSource
	l.sources.AddSource(fileSource.source)
}

func (l *Launcher) unscheduleFileSource(containerID string) {
	if sourcePair, exists := l.fileSourcesByContainer[containerID]; exists {
		sourcePair.info.RemoveMessage(containerID)
		delete(l.fileSourcesByContainer, containerID)
		l.sources.RemoveSource(sourcePair.source)
	}
}

func (l *Launcher) startSocketTailer(container *Container, source *config.LogSource) {
	containerID := container.service.Identifier
	if _, isTailed := l.getTailer(containerID); isTailed {
		log.Warnf("Can't tail twice the same container: %v", dockerutilpkg.ShortContainerID(containerID))
		return
	}
	dockerutil, err := dockerutilpkg.GetDockerUtil()
	if err != nil {
		log.Warnf("Could not use docker client, logs for container %s won’t be collected: %v", containerID, err)
		return
	}
	// overridenSource == source if the containerCollectAll option is not activated or the container has AD labels
	overridenSource := l.overrideSource(container, source)
	tailer := tailer.NewTailer(dockerutil, containerID, overridenSource, l.pipelineProvider.NextPipelineChan(), l.erroredContainerID, l.readTimeout)

	// compute the offset to prevent from missing or duplicating logs
	since, err := Since(l.registry, tailer.Identifier())
	if err != nil {
		log.Warnf("Could not recover tailing from last committed offset %v: %v", dockerutilpkg.ShortContainerID(containerID), err)
	}

	// start the tailer
	err = tailer.Start(since)
	if err != nil {
		log.Warnf("Could not start tailer %s: %v", containerID, err)
		return
	}
	source.AddInput(containerID)

	// keep the tailer in track to stop it later on
	l.addTailer(containerID, tailer)
}

// stopTailer stops the tailer matching the containerID.
func (l *Launcher) stopTailer(containerID string) {
	if l.tailFromFile {
		l.unscheduleFileSource(containerID)
	} else {
		l.stopSocketTailer(containerID)
	}
}

func (l *Launcher) stopSocketTailer(containerID string) {
	if tailer, isTailed := l.getTailer(containerID); isTailed {
		// No-op if the tailer source came from AD
		if l.collectAllSource != nil {
			l.collectAllSource.RemoveInput(containerID)
			l.collectAllInfo.RemoveMessage(containerID)
		}
		go tailer.Stop()
		l.removeTailer(containerID)
	}
}

func (l *Launcher) restartTailer(containerID string) {
	// It should never happen
	if l.tailFromFile {
		return
	}
	backoffDuration := backoffInitialDuration
	cumulatedBackoff := 0 * time.Second
	var source *config.LogSource

	if oldTailer, exists := l.getTailer(containerID); exists {
		source = oldTailer.Source
		if l.collectAllSource != nil {
			l.collectAllSource.RemoveInput(containerID)
			l.collectAllInfo.RemoveMessage(containerID)
		}
		oldTailer.Stop()
		l.removeTailer(containerID)
	} else {
		log.Warnf("Unable to restart tailer, old source not found, keeping previous one, container: %s", containerID)
		return
	}

	dockerutil, err := dockerutilpkg.GetDockerUtil()
	if err != nil {
		// This cannot happen since, if we have a tailer to restart, it means that we created
		// it earlier and we couldn't have created it if the docker client wasn't initialized.
		log.Warnf("Could not use docker client, logs for container %s won’t be collected: %v", containerID, err)
		return
	}
	tailer := tailer.NewTailer(dockerutil, containerID, source, l.pipelineProvider.NextPipelineChan(), l.erroredContainerID, l.readTimeout)

	// compute the offset to prevent from missing or duplicating logs
	since, err := Since(l.registry, tailer.Identifier())
	if err != nil {
		log.Warnf("Could not recover last committed offset for container %v: %v", dockerutilpkg.ShortContainerID(containerID), err)
	}

	for {
		if backoffDuration > backoffMaxDuration {
			log.Warnf("Could not resume tailing container %v", dockerutilpkg.ShortContainerID(containerID))
			return
		}

		// start the tailer
		err = tailer.Start(since)
		if err != nil {
			log.Warnf("Could not start tailer for container %v: %v", dockerutilpkg.ShortContainerID(containerID), err)
			time.Sleep(backoffDuration)
			cumulatedBackoff += backoffDuration
			backoffDuration *= 2
			continue
		}
		// keep the tailer in track to stop it later on
		l.addTailer(containerID, tailer)
		source.AddInput(containerID)
		return
	}
}

func (l *Launcher) addTailer(containerID string, tailer *tailer.Tailer) {
	l.lock.Lock()
	l.tailers[containerID] = tailer
	l.lock.Unlock()
}

func (l *Launcher) removeTailer(containerID string) {
	l.lock.Lock()
	delete(l.tailers, containerID)
	l.lock.Unlock()
}

func (l *Launcher) getTailer(containerID string) (*tailer.Tailer, bool) {
	l.lock.Lock()
	defer l.lock.Unlock()
	tailer, exist := l.tailers[containerID]
	return tailer, exist
}
