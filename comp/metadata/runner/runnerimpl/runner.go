// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runnerimpl

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/metadata/runner"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"go.uber.org/fx"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newRunner))
}

// MetadataProvider is the provider for metadata
type MetadataProvider func(context.Context) time.Duration

// PriorityMetadataProvider is the provider for metadata that needs to be execute at start time of the agent.
// Right now the agent needs the host metadata provider to execute first to ensure host tags are being reported correctly.
// This is a temporary workaorund until we figure a more permanent solution in the backend
// If you need to use this provide please contact the agent-shared-components team
type PriorityMetadataProvider func(context.Context) time.Duration

type runnerImpl struct {
	log    log.Component
	config config.Component

	providers         []MetadataProvider
	priorityProviders []PriorityMetadataProvider

	wg       sync.WaitGroup
	stopChan chan struct{}
}

type dependencies struct {
	fx.In

	Log    log.Component
	Config config.Component

	Providers         []optional.Option[MetadataProvider]         `group:"metadata_provider"`
	PriorityProviders []optional.Option[PriorityMetadataProvider] `group:"metadata_priority_provider"`
}

// Provider represents the callback from a metada provider. This is returned by 'NewProvider' helper.
type Provider struct {
	fx.Out

	Callback optional.Option[MetadataProvider] `group:"metadata_provider"`
}

// PriorityProvider represents the callback from a priority metada provider. This is returned by 'NewPriorityProvider' helper.
type PriorityProvider struct {
	fx.Out

	Callback optional.Option[PriorityMetadataProvider] `group:"metadata_priority_provider"`
}

// NewProvider registers a new metadata provider by adding a callback to the runner.
func NewProvider(callback MetadataProvider) Provider {
	return Provider{
		Callback: optional.NewOption[MetadataProvider](callback),
	}
}

// NewPriorityProvider registers a new metadata provider by adding a callback to the runner.
func NewPriorityProvider(callback PriorityMetadataProvider) PriorityProvider {
	return PriorityProvider{
		Callback: optional.NewOption[PriorityMetadataProvider](callback),
	}
}

// NewEmptyProvider returns a empty provider which is not going to register anything. This is useful for providers that
// can be enabled/disabled through configuration.
func NewEmptyProvider() Provider {
	return Provider{
		Callback: optional.NewNoneOption[MetadataProvider](),
	}
}

// createRunner instantiates a runner object
func createRunner(deps dependencies) *runnerImpl {
	providers := []MetadataProvider{}
	priorityProviders := []PriorityMetadataProvider{}

	nonNilProviders := fxutil.GetAndFilterGroup(deps.Providers)
	nonNilPriorityProviders := fxutil.GetAndFilterGroup(deps.PriorityProviders)

	for _, optionaP := range nonNilProviders {
		if p, isSet := optionaP.Get(); isSet {
			providers = append(providers, p)
		}
	}

	for _, optionaP := range nonNilPriorityProviders {
		if p, isSet := optionaP.Get(); isSet {
			priorityProviders = append(priorityProviders, p)
		}
	}

	return &runnerImpl{
		log:               deps.Log,
		config:            deps.Config,
		providers:         providers,
		priorityProviders: priorityProviders,
		stopChan:          make(chan struct{}),
	}
}

func newRunner(lc fx.Lifecycle, deps dependencies) runner.Component {
	r := createRunner(deps)

	if deps.Config.GetBool("enable_metadata_collection") {
		// We rely on FX to start and stop the metadata runner
		lc.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				return r.start()
			},
			OnStop: func(ctx context.Context) error {
				return r.stop()
			},
		})
	} else {
		deps.Log.Info("Metadata collection is disabled, only do this if another agent/dogstatsd is running on this host")
	}
	return r
}

// handleProvider runs a provider at regular interval until the runner is stopped
func (r *runnerImpl) handleProvider(p func(context.Context) time.Duration) {
	r.log.Debugf("Starting runner for MetadataProvider %#v", p)
	r.wg.Add(1)

	intervalChan := make(chan time.Duration)
	var interval time.Duration

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		r.log.Debugf("stopping runner for MetadataProvider %#v", p)
		r.wg.Done()
	}()

	for {
		go func(intervalChan chan time.Duration) {
			intervalChan <- p(ctx)
		}(intervalChan)

		select {
		case interval = <-intervalChan:
		case <-r.stopChan:
			cancel()
			return
		}

		select {
		case <-time.After(interval):
		case <-r.stopChan:
			return
		}
	}
}

// start is called by FX when the application starts. Lifecycle hooks are blocking and called sequencially. We should
// not block here.
func (r *runnerImpl) start() error {
	r.log.Debugf("Starting metadata runner with %d priority providers and %d regular providers", len(r.priorityProviders), len(r.providers))

	go func() {
		for _, priorityProvider := range r.priorityProviders {
			// Execute synchronously the priority provider
			priorityProvider(context.Background())

			go r.handleProvider(priorityProvider)
		}

		for _, provider := range r.providers {
			go r.handleProvider(provider)
		}
	}()

	return nil
}

// stop is called by FX when the application stops. Lifecycle hooks are blocking and called sequencially. We should
// not block here.
func (r *runnerImpl) stop() error {
	r.log.Debugf("Stopping metadata runner")
	close(r.stopChan)
	r.wg.Wait()
	return nil
}
