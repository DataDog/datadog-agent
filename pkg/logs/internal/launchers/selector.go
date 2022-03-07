// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package launchers

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

// Factory is a retryable wrapper that creates a new launcher.
type Factory struct {
	// IsAvailable indicates whether this factory is capable of creating a launcher;
	// if not (false), then it returns a Retrier to indicate when the selector should
	// call IsAvailable again.  The selector may call IsAvailable before this time.
	IsAvailable func() (bool, *retry.Retrier)

	// NewLauncher creates a new launcher.  Once this has been called, the selector
	// proxies to this launcher.
	NewLauncher func() Launcher
}

// LauncherSelector tries to select a launcher from a set of launcher factories, proxying
// to the first one that becomes available.
//
// A factory earlier in the list of factories is preferred, if multiple factories are available
// at the same time.
type LauncherSelector struct {
	// factories are the launcher factories, in order by priority
	factories []Factory

	// activeLauncher is the currently selected launcher.  Once this value is non-nil,
	// it does not change.
	activeLauncher Launcher

	// sourceProvider is the Start argument to be passed to the selected launcher.
	sourceProvider SourceProvider

	// pipelineProvider is the Start argument to be passed to the selected launcher.
	pipelineProvider pipeline.Provider

	// registry is the Start argument to be passed to the selected launcher.
	registry auditor.Registry

	// when stop is true, the selector will stop trying to create a launcher.
	stop bool

	// Mutex protects `activeLauncher` and `stop` from concurrent access.
	sync.Mutex
}

// NewLauncherSelector creates a new launcher
func NewLauncherSelector(containerLaunchers []Factory) *LauncherSelector {
	return &LauncherSelector{
		factories: containerLaunchers,
	}
}

func (l *LauncherSelector) launch(launchable Factory) {
	launcher := launchable.NewLauncher()
	l.activeLauncher = launcher
	l.activeLauncher.Start(l.sourceProvider, l.pipelineProvider, l.registry)
}

func (l *LauncherSelector) shouldRetry() (bool, time.Duration) {
	var retryer *retry.Retrier
	for _, launchable := range l.factories {
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

// Start implements Launcher#Start.
//
// It begins trying to create a launcher, proxying the Start call to the selected launcher.
func (l *LauncherSelector) Start(sourceProvider SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry) {
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
func (l *LauncherSelector) Stop() {
	l.Lock()
	defer l.Unlock()
	l.stop = true
	if l.activeLauncher != nil {
		l.activeLauncher.Stop()
	}
}
