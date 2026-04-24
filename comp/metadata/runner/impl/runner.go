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

type runnerImpl struct {
	log    log.Component
	config config.Component

	providers []runner.MetadataProvider

	wg       sync.WaitGroup
	stopChan chan struct{}
}

// Requires defines the dependencies for the runner component
type Requires struct {
	compdef.In

	Lc     compdef.Lifecycle
	Log    log.Component
	Config config.Component

	Providers []runner.MetadataProvider `group:"metadata_provider"`
}

// Provides defines the output of the runner component
type Provides struct {
	compdef.Out

	Comp runner.Component
}

// createRunner instantiates a runner object
func createRunner(deps Requires) *runnerImpl {
	return &runnerImpl{
		log:       deps.Log,
		config:    deps.Config,
		providers: runner.GetAndFilterProviders(deps.Providers),
		stopChan:  make(chan struct{}),
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

// handleProvider runs a provider at regular interval until the runner is stopped
func (r *runnerImpl) handleProvider(p runner.MetadataProvider) {
	r.log.Debugf("Starting runner for MetadataProvider %#v", p)

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

// start is called by FX when the application starts. Lifecycle hooks are blocking and called sequentially. We should
// not block here.
func (r *runnerImpl) start() error {
	r.log.Debugf("Starting metadata runner with %d providers", len(r.providers))

	for _, provider := range r.providers {
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			r.handleProvider(provider)
		}()
	}

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
