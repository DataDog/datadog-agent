// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// configPoller keeps track of the configurations loaded by a certain
// `ConfigProvider` and whether it should be polled or not.
type configPoller struct {
	provider types.ConfigProvider

	isRunning bool

	canPoll      bool
	pollInterval time.Duration

	stopChan chan struct{}

	configsMu      sync.Mutex
	configs        map[uint64]integration.Config
	telemetryStore *telemetry.Store
}

func newConfigPoller(provider types.ConfigProvider, canPoll bool, interval time.Duration, telemetryStore *telemetry.Store) *configPoller {
	return &configPoller{
		provider:       provider,
		configs:        make(map[uint64]integration.Config),
		canPoll:        canPoll,
		pollInterval:   interval,
		stopChan:       make(chan struct{}),
		telemetryStore: telemetryStore,
	}
}

// stop stops the provider descriptor if it's polling
func (cp *configPoller) stop() {
	if !cp.canPoll || cp.isRunning {
		return
	}
	cp.stopChan <- struct{}{}
	cp.isRunning = false
}

// start starts polling the provider descriptor. It blocks until the provider
// returns all the known configs.
func (cp *configPoller) start(ctx context.Context, ac *AutoConfig) {
	switch provider := cp.provider.(type) {
	case types.StreamingConfigProvider:
		cp.stopChan = make(chan struct{})

		ch := make(chan struct{})
		go cp.stream(ch, provider, ac)
		<-ch

	case types.CollectingConfigProvider:
		cp.collectOnce(ctx, provider, ac)

		if !cp.canPoll {
			return
		}

		go cp.poll(provider, ac)
	default:
		panic(fmt.Sprintf("provider %q does not implement StreamingConfigProvider nor CollectingConfigProvider", provider.String()))
	}
}

// stream streams config from the corresponding config provider
func (cp *configPoller) stream(ch chan struct{}, provider types.StreamingConfigProvider, ac *AutoConfig) {
	var ranOnce bool
	ctx, cancel := context.WithCancel(context.Background())
	changesCh := provider.Stream(ctx)
	healthHandle := health.RegisterLiveness(fmt.Sprintf("ad-config-provider-%s", cp.provider.String()))

	cp.isRunning = true

	for {
		select {
		case <-healthHandle.C:

		case <-cp.stopChan:
			err := healthHandle.Deregister()
			if err != nil {
				log.Errorf("error de-registering health check: %s", err)
			}

			cancel()

			return

		case changes := <-changesCh:
			if !changes.IsEmpty() {
				log.Infof("%v provider: collected %d new configurations, removed %d", provider, len(changes.Schedule), len(changes.Unschedule))

				ac.processRemovedConfigs(changes.Unschedule)

				for _, added := range changes.Schedule {
					added.Provider = cp.provider.String()
					resolvedChanges := ac.processNewConfig(added)
					ac.applyChanges(resolvedChanges)
				}
			}

			if !ranOnce {
				close(ch)
				ranOnce = true
			}
		}
	}
}

// poll polls config of the corresponding config provider
func (cp *configPoller) poll(provider types.CollectingConfigProvider, ac *AutoConfig) {
	ctx, cancel := context.WithCancel(context.Background())
	ticker := time.NewTicker(cp.pollInterval)
	healthHandle := health.RegisterLiveness(fmt.Sprintf("ad-config-provider-%s", cp.provider.String()))

	cp.isRunning = true

	for {
		select {
		case healthDeadline := <-healthHandle.C:
			cancel()
			ctx, cancel = context.WithDeadline(context.Background(), healthDeadline)
		case <-cp.stopChan:
			err := healthHandle.Deregister()
			if err != nil {
				log.Errorf("error de-registering health check: %s", err)
			}

			cancel()
			ticker.Stop()
			return
		case <-ticker.C:
			upToDate, err := provider.IsUpToDate(ctx)
			if err != nil {
				log.Errorf("Cache processing of %v configuration provider failed: %v", cp.provider, err)
				continue
			}

			if upToDate {
				log.Debugf("No modifications in the templates stored in %v configuration provider", cp.provider)
				continue
			}

			cp.collectOnce(ctx, provider, ac)
		}
	}
}

func (cp *configPoller) collectOnce(ctx context.Context, provider types.CollectingConfigProvider, ac *AutoConfig) {
	// retrieve the list of newly added configurations as well
	// as removed configurations
	newConfigs, removedConfigs := cp.collect(ctx, provider)
	if len(newConfigs) > 0 || len(removedConfigs) > 0 {
		log.Infof("%v provider: collected %d new configurations, removed %d", cp.provider, len(newConfigs), len(removedConfigs))
	} else {
		log.Debugf("%v provider: no configuration change", cp.provider)
	}

	// Process removed configs first to handle the case where a
	// container churn would result in the same configuration hash.
	ac.processRemovedConfigs(removedConfigs)

	for _, config := range newConfigs {
		if _, ok := cp.provider.(*providers.FileConfigProvider); ok {
			// JMX checks can have 2 YAML files: one containing the
			// metrics to collect, one containing the instance
			// configuration. If the file provider finds any of
			// these metric YAMLs, we store them in a map for
			// future access
			if config.MetricConfig != nil {
				// We don't want to save metric files, it's enough to store them in the map
				ac.store.setJMXMetricsForConfigName(config.Name, config.MetricConfig)
				continue
			}

			// Clear any old errors if a valid config file is found
			errorStats.removeConfigError(config.Name)
		}

		config.Provider = cp.provider.String()
		changes := ac.processNewConfig(config)
		ac.applyChanges(changes)
	}

}

// collect is just a convenient wrapper to fetch configurations from a provider and
// see what changed from the last time we called Collect().
func (cp *configPoller) collect(ctx context.Context, provider types.CollectingConfigProvider) ([]integration.Config, []integration.Config) {
	start := time.Now()
	defer func() {
		if cp.telemetryStore != nil {
			cp.telemetryStore.PollDuration.Observe(time.Since(start).Seconds(), cp.provider.String())
		}
	}()

	fetched, err := provider.Collect(ctx)
	if err != nil {
		log.Errorf("Unable to collect configurations from provider %s: %s", cp.provider, err)
		return nil, nil
	}

	return cp.storeAndDiffConfigs(fetched)
}

func (cp *configPoller) storeAndDiffConfigs(configs []integration.Config) ([]integration.Config, []integration.Config) {
	cp.configsMu.Lock()
	defer cp.configsMu.Unlock()

	var newConf []integration.Config
	var removedConf []integration.Config

	// We allocate a new map. We could do without it with a bit more processing
	// but it allows to free some memory if number of collected configs varies a lot
	fetchedMap := make(map[uint64]integration.Config, len(configs))
	for _, c := range configs {
		cHash := c.FastDigest()
		fetchedMap[cHash] = c
		if _, found := cp.configs[cHash]; found {
			delete(cp.configs, cHash)
		} else {
			newConf = append(newConf, c)
		}
	}

	for _, c := range cp.configs {
		removedConf = append(removedConf, c)
	}
	cp.configs = fetchedMap

	return newConf, removedConf
}
