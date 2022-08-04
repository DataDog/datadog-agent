// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscovery

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/telemetry"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// configPoller keeps track of the configurations loaded by a certain
// `ConfigProvider` and whether it should be polled or not.
type configPoller struct {
	provider     providers.ConfigProvider
	configs      map[uint64]integration.Config
	configsMu    sync.Mutex
	canPoll      bool
	isPolling    bool
	pollInterval time.Duration
	stopChan     chan struct{}
	healthHandle *health.Handle
}

func newConfigPoller(provider providers.ConfigProvider, canPoll bool, interval time.Duration) *configPoller {
	return &configPoller{
		provider:     provider,
		configs:      make(map[uint64]integration.Config),
		canPoll:      canPoll,
		pollInterval: interval,
	}
}

// stop stops the provider descriptor if it's polling
func (cp *configPoller) stop() {
	if !cp.canPoll || cp.isPolling {
		return
	}
	cp.stopChan <- struct{}{}
	cp.isPolling = false
}

// start starts polling the provider descriptor
func (cp *configPoller) start(ctx context.Context, ac *AutoConfig) {
	_, isStreaming := cp.provider.(providers.StreamingConfigProvider)

	if !isStreaming {
		cp.collectOnce(ctx, ac)
	}

	if !cp.canPoll {
		return
	}

	cp.stopChan = make(chan struct{})
	cp.healthHandle = health.RegisterLiveness(fmt.Sprintf("ad-config-provider-%s", cp.provider.String()))
	cp.isPolling = true

	if isStreaming {
		ch := make(chan struct{})
		go cp.stream(ch, ac)
		<-ch
	} else {
		go cp.poll(ac)
	}
}

// stream streams config from the corresponding config provider
func (cp *configPoller) stream(ch chan struct{}, ac *AutoConfig) {
	var ranOnce bool
	ctx, cancel := context.WithCancel(context.Background())
	changesCh := cp.provider.(providers.StreamingConfigProvider).Stream(ctx)

	for {
		select {
		case <-cp.healthHandle.C:

		case <-cp.stopChan:
			err := cp.healthHandle.Deregister()
			if err != nil {
				log.Errorf("error de-registering health check: %s", err)
			}

			cancel()

			return

		case changes := <-changesCh:
			ac.applyChanges(changes)

			if !ranOnce {
				close(ch)
				ranOnce = true
			}
		}
	}
}

// poll polls config of the corresponding config provider
func (cp *configPoller) poll(ac *AutoConfig) {
	ctx, cancel := context.WithCancel(context.Background())
	ticker := time.NewTicker(cp.pollInterval)
	for {
		select {
		case healthDeadline := <-cp.healthHandle.C:
			cancel()
			ctx, cancel = context.WithDeadline(context.Background(), healthDeadline)
		case <-cp.stopChan:
			err := cp.healthHandle.Deregister()
			if err != nil {
				log.Errorf("error de-registering health check: %s", err)
			}

			cancel()
			ticker.Stop()
			return
		case <-ticker.C:
			upToDate, err := cp.provider.IsUpToDate(ctx)
			if err != nil {
				log.Errorf("Cache processing of %v configuration provider failed: %v", cp.provider, err)
				continue
			}

			if upToDate {
				log.Debugf("No modifications in the templates stored in %v configuration provider", cp.provider)
				continue
			}

			cp.collectOnce(ctx, ac)
		}
	}
}

func (cp *configPoller) collectOnce(ctx context.Context, ac *AutoConfig) {
	// retrieve the list of newly added configurations as well
	// as removed configurations
	newConfigs, removedConfigs := cp.collect(ctx)
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
func (cp *configPoller) collect(ctx context.Context) ([]integration.Config, []integration.Config) {
	start := time.Now()
	defer func() {
		telemetry.PollDuration.Observe(time.Since(start).Seconds(), cp.provider.String())
	}()

	fetched, err := cp.provider.Collect(ctx)
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

func (cp *configPoller) getConfigs() []integration.Config {
	cp.configsMu.Lock()
	defer cp.configsMu.Unlock()

	configs := make([]integration.Config, 0, len(cp.configs))

	for _, cfg := range cp.configs {
		configs = append(configs, cfg)
	}

	return configs
}
