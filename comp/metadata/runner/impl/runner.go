// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package runnerimpl implements a component to generate metadata payload at the right interval.
package runnerimpl

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	runner "github.com/DataDog/datadog-agent/comp/metadata/runner/def"
)

const defaultPrimaryWaitTimeout = 15 * time.Second

type runnerImpl struct {
	log    log.Component
	config config.Component

	primaryProviders   []runner.MetadataProvider
	providers          []runner.MetadataProvider
	primaryWaitTimeout time.Duration

	wg       sync.WaitGroup
	stopChan chan struct{}
}

// Requires defines the dependencies for the runner component
type Requires struct {
	compdef.In

	Lc     compdef.Lifecycle
	Log    log.Component
	Config config.Component

	PrimaryProviders []runner.MetadataProvider `group:"metadata_provider_primary"`
	Providers        []runner.MetadataProvider `group:"metadata_provider"`
}

// Provides defines the output of the runner component
type Provides struct {
	compdef.Out

	Comp runner.Component
}

// createRunner instantiates a runner object
func createRunner(deps Requires) *runnerImpl {
	return &runnerImpl{
		log:                deps.Log,
		config:             deps.Config,
		primaryProviders:   runner.GetAndFilterProviders(deps.PrimaryProviders),
		providers:          runner.GetAndFilterProviders(deps.Providers),
		primaryWaitTimeout: defaultPrimaryWaitTimeout,
		stopChan:           make(chan struct{}),
	}
}

// NewComponent creates a new runner component
func NewComponent(deps Requires) Provides {
	r := createRunner(deps)

	if deps.Config.GetBool("enable_metadata_collection") {
		// We rely on FX to start and stop the metadata runner
		deps.Lc.Append(compdef.Hook{
			OnStart: func(_ context.Context) error {
				return r.start()
			},
			OnStop: func(_ context.Context) error {
				return r.stop()
			},
		})
	} else {
		deps.Log.Info("Metadata collection is disabled, only do this if another agent/dogstatsd is running on this host")
	}
	return Provides{Comp: r}
}

// handleProvider runs a provider at regular interval until the runner is stopped.
// If onFirstRun is non-nil, it is called once after the first collection returns.
func (r *runnerImpl) handleProvider(p runner.MetadataProvider, onFirstRun func()) {
	r.log.Debugf("Starting runner for MetadataProvider %#v", p)

	var firstRunOnce sync.Once
	markDone := func() {
		if onFirstRun != nil {
			firstRunOnce.Do(onFirstRun)
		}
	}
	defer markDone()

	intervalChan := make(chan time.Duration)
	var interval time.Duration

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		r.log.Debugf("stopping runner for MetadataProvider %#v", p)
	}()

	for {
		go func(intervalChan chan time.Duration) {
			intervalChan <- p(ctx)
		}(intervalChan)

		select {
		case interval = <-intervalChan:
			markDone()
		case <-r.stopChan:
			cancel()

			// Wait for the provider to finish to avoid it being interrupted when the agent stops
			// as that could cause a corruption
			// Still stop after some max timeout to avoid blocking the agent stop entirely
			select {
			case <-intervalChan:
			case <-time.After(r.config.GetDuration("metadata_provider_stop_timeout")):
			}
			return
		}

		select {
		case <-time.After(interval):
		case <-r.stopChan:
			return
		}
	}
}

func (r *runnerImpl) startProviders(providers []runner.MetadataProvider, onFirstRun func()) {
	for _, provider := range providers {
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			r.handleProvider(provider, onFirstRun)
		}()
	}
}

// start is called by FX when the application starts. Lifecycle hooks are blocking and called sequentially. We should
// not block here.
func (r *runnerImpl) start() error {
	r.log.Debugf("Starting metadata runner with %d primary and %d regular providers", len(r.primaryProviders), len(r.providers))

	if len(r.primaryProviders) == 0 {
		r.startProviders(r.providers, nil)
		return nil
	}

	primaryDone := make(chan struct{})
	var primaryWG sync.WaitGroup
	primaryWG.Add(len(r.primaryProviders))
	r.startProviders(r.primaryProviders, primaryWG.Done)
	go func() {
		primaryWG.Wait()
		close(primaryDone)
	}()

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		timer := time.NewTimer(r.primaryWaitTimeout)
		defer timer.Stop()
		select {
		case <-primaryDone:
		case <-timer.C:
			r.log.Warnf("Timed out after %s waiting for primary metadata providers; starting regular providers", r.primaryWaitTimeout)
		case <-r.stopChan:
			return
		}
		select {
		case <-r.stopChan:
			return
		default:
			r.startProviders(r.providers, nil)
		}
	}()

	return nil
}

// stop is called by FX when the application stops. Lifecycle hooks are blocking and called sequentially. We should
// not block here.
func (r *runnerImpl) stop() error {
	r.log.Debugf("Stopping metadata runner")
	close(r.stopChan)
	r.wg.Wait()
	return nil
}
