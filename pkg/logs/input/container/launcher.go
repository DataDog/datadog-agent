// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package container

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/restart"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

// Launchable is a retryable wrapper for a restartable
type Launchable struct {
	IsAvailable func() (bool, *retry.Retrier)
	Launcher    func() restart.Restartable
}

// Launcher tries to select a container launcher and retry on failure
type Launcher struct {
	containerLaunchables []Launchable
	activeLauncher       restart.Restartable
	stopped              chan struct{}
	hasStopped           bool
	lock                 sync.Mutex
}

// NewLauncher creates a new launcher
func NewLauncher(containerLaunchers []Launchable) *Launcher {
	return &Launcher{
		containerLaunchables: containerLaunchers,
		stopped:              make(chan struct{}),
	}
}

func (l *Launcher) launch(launchable Launchable) {
	l.lock.Lock()
	launcher := launchable.Launcher()
	if launcher == nil {
		launcher = NewNoopLauncher()
	}
	l.activeLauncher = launcher
	l.activeLauncher.Start()
	l.hasStopped = true
	l.lock.Unlock()
}

// Start starts the launcher
func (l *Launcher) Start() {
	// If we are restarting, start up the active launcher since we already picked one from a previous run
	l.lock.Lock()
	if l.activeLauncher != nil {
		l.activeLauncher.Start()
		l.lock.Unlock()
		return
	}
	l.lock.Unlock()

	// Try to select a launcher
	go func() {
		timer := time.NewTimer(0)
		for {
			select {
			case <-l.stopped:
				log.Info("Got stop signal - stopping")
				return
			case <-timer.C:
				var retryer *retry.Retrier
				for _, launchable := range l.containerLaunchables {
					ok, rt := launchable.IsAvailable()
					if ok {
						l.launch(launchable)
						return
					}
					// Hold on to the retrier with the longest interval
					if retryer == nil || (rt != nil && retryer.NextRetry().Before(rt.NextRetry())) {
						retryer = rt
					}
				}
				if retryer == nil {
					log.Info("Nothing to retry - stopping")
					l.hasStopped = true
					return
				}
				nextRetry := time.Until(retryer.NextRetry())
				timer = time.NewTimer(nextRetry)
				log.Infof("Could not find an available a container launcher - will try again in %s", nextRetry.Truncate(time.Second))
			}
		}
	}()
}

// Stop stops the launcher
func (l *Launcher) Stop() {
	defer l.lock.Unlock()
	l.lock.Lock()
	if !l.hasStopped {
		l.stopped <- struct{}{}
	}
	if l.activeLauncher != nil {
		l.activeLauncher.Stop()
	}
}

// NewLauncher returns a new container launcher depending on the environment.
// By default returns a docker launcher if the docker socket is mounted and fallback to
// a kubernetes launcher if '/var/log/pods' is mounted ; this behaviour is reversed when
// kubernetesCollectFromFiles is enabled.
// If dockerCollectFromFiles is enabled the docker launcher will first attempt to tail
// containers from file instead of the docker socket if '/var/lib/docker/containers'
// is mounted.
// If none of those volumes are mounted, returns a lazy docker launcher with a retrier to handle the cases
// where docker is started after the agent.
// dockerReadTimeout is a configurable read timeout for the docker client.
// func OldNewLauncher(collectAll bool,
// 	kubernetesCollectFromFiles bool,
// 	dockerCollectFromFiles bool,
// 	dockerForceCollectFromFile bool,
// 	dockerReadTimeout time.Duration,
// 	sources *config.LogSources,
// 	services *service.Services,
// 	pipelineProvider pipeline.Provider,
// 	registry auditor.Registry) restart.Restartable {
// 	var (
// 		launcher restart.Restartable
// 		err      error
// 	)

// 	if kubernetesCollectFromFiles {
// 		launcher, err = kubernetes.NewLauncher(sources, services, collectAll)
// 		if err == nil {
// 			log.Info("Kubernetes launcher initialized")
// 			return launcher
// 		}
// 		log.Infof("Could not setup the kubernetes launcher: %v", err)

// 		launcher, err = docker.NewLauncher(dockerReadTimeout, sources, services, pipelineProvider, registry, false, dockerCollectFromFiles, dockerForceCollectFromFile)
// 		if err == nil {
// 			log.Info("Docker launcher initialized")
// 			return launcher
// 		}
// 		log.Infof("Could not setup the docker launcher: %v", err)
// 	} else {
// 		launcher, err = docker.NewLauncher(dockerReadTimeout, sources, services, pipelineProvider, registry, false, dockerCollectFromFiles, dockerForceCollectFromFile)
// 		if err == nil {
// 			log.Info("Docker launcher initialized")
// 			return launcher
// 		}
// 		log.Infof("Could not setup the docker launcher: %v", err)

// 		launcher, err = kubernetes.NewLauncher(sources, services, collectAll)
// 		if err == nil {
// 			log.Info("Kubernetes launcher initialized")
// 			return launcher
// 		}
// 		log.Infof("Could not setup the kubernetes launcher: %v", err)
// 	}

// 	launcher, err = docker.NewLauncher(dockerReadTimeout, sources, services, pipelineProvider, registry, true, dockerCollectFromFiles, dockerForceCollectFromFile)
// 	if err != nil {
// 		log.Warnf("Could not setup the docker launcher: %v. Will not be able to collect container logs", err)
// 		return NewNoopLauncher()
// 	}

// 	log.Infof("Container logs won't be collected unless a docker daemon is eventually started")

// 	return launcher
// }
