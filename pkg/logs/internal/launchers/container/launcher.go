// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package container

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

// Launchable is a retryable wrapper for a restartable
type Launchable struct {
	IsAvailable func() (bool, *retry.Retrier)
	Launcher    func() launchers.Launcher
}

// Launcher tries to select a container launcher and retry on failure
type Launcher struct {
	containerLaunchables []Launchable
	activeLauncher       launchers.Launcher
	sourceProvider       launchers.SourceProvider
	pipelineProvider     pipeline.Provider
	registry             auditor.Registry
	stop                 bool
	sync.Mutex
}

// NewLauncher creates a new launcher
func NewLauncher(containerLaunchers []Launchable) *Launcher {
	return &Launcher{
		containerLaunchables: containerLaunchers,
	}
}

func (l *Launcher) launch(launchable Launchable) {
	launcher := launchable.Launcher()
	if launcher == nil {
		launcher = NewNoopLauncher()
	}
	l.activeLauncher = launcher
	l.activeLauncher.Start(l.sourceProvider, l.pipelineProvider, l.registry)
}

func (l *Launcher) shouldRetry() (bool, time.Duration) {
	var retryer *retry.Retrier
	for _, launchable := range l.containerLaunchables {
		ok, rt := launchable.IsAvailable()
		if ok {
			l.launch(launchable)
			return false, 0
		}
		// Hold on to the retrier with the longest interval
		if retryer == nil || (rt != nil && retryer.NextRetry().Before(rt.NextRetry())) {
			retryer = rt
		}
	}
	if retryer == nil {
		log.Info("Nothing to retry - stopping")
		return false, 0
	}
	nextRetry := time.Until(retryer.NextRetry())
	log.Infof("Could not find an available a container launcher - will try again in %s", nextRetry.Truncate(time.Second))
	return true, nextRetry

}

// Start starts the launcher
func (l *Launcher) Start(sourceProvider launchers.SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry) {
	l.sourceProvider = sourceProvider
	l.pipelineProvider = pipelineProvider
	l.registry = registry

	// If we are restarting, start up the active launcher since we already picked one from a previous run
	l.Lock()
	if l.activeLauncher != nil {
		l.stop = true
		l.activeLauncher.Start(l.sourceProvider, l.pipelineProvider, l.registry)
		l.Unlock()
		return
	}
	l.stop = false
	l.Unlock()

	// Try to select a launcher
	go func() {
		for {
			l.Lock()
			if l.stop {
				l.Unlock()
				return
			}
			shouldRetry, nextRetry := l.shouldRetry()
			l.stop = !shouldRetry
			if !shouldRetry {
				l.Unlock()
				return
			}
			l.Unlock()
			<-time.After(nextRetry)
		}
	}()
}

// Stop stops the launcher
func (l *Launcher) Stop() {
	l.Lock()
	defer l.Unlock()
	l.stop = true
	if l.activeLauncher != nil {
		l.activeLauncher.Stop()
	}
}
