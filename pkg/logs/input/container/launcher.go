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
