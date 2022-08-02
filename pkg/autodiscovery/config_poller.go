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
func (pd *configPoller) stop() {
	if !pd.canPoll || pd.isPolling {
		return
	}
	pd.stopChan <- struct{}{}
	pd.isPolling = false
}

// start starts polling the provider descriptor
func (pd *configPoller) start(ctx context.Context, ac *AutoConfig) {
	_, isStreaming := pd.provider.(providers.StreamingConfigProvider)

	if !isStreaming {
		pd.pollOnce(ctx, ac)
	}

	if !pd.canPoll {
		return
	}

	pd.stopChan = make(chan struct{})
	pd.healthHandle = health.RegisterLiveness(fmt.Sprintf("ad-config-provider-%s", pd.provider.String()))
	pd.isPolling = true

	if isStreaming {
		ch := make(chan struct{})
		go pd.stream(ch, ac)
		<-ch
	} else {
		go pd.poll(ac)
	}
}

// stream streams config from the corresponding config provider
func (pd *configPoller) stream(ch chan struct{}, ac *AutoConfig) {
	var ranOnce bool
	ctx, cancel := context.WithCancel(context.Background())
	changesCh := pd.provider.(providers.StreamingConfigProvider).Stream(ctx)

	for {
		select {
		case <-pd.healthHandle.C:

		case <-pd.stopChan:
			err := pd.healthHandle.Deregister()
			if err != nil {
				log.Errorf("error de-registering health check: %s", err)
			}

			cancel()

			return

		case changes := <-changesCh:
			ac.applyChanges(configChanges{
				schedule:   changes.Schedule,
				unschedule: changes.Unschedule,
			})

			if !ranOnce {
				close(ch)
				ranOnce = true
			}
		}
	}
}

// poll polls config of the corresponding config provider
func (pd *configPoller) poll(ac *AutoConfig) {
	ctx, cancel := context.WithCancel(context.Background())
	ticker := time.NewTicker(pd.pollInterval)
	for {
		select {
		case healthDeadline := <-pd.healthHandle.C:
			cancel()
			ctx, cancel = context.WithDeadline(context.Background(), healthDeadline)
		case <-pd.stopChan:
			err := pd.healthHandle.Deregister()
			if err != nil {
				log.Errorf("error de-registering health check: %s", err)
			}

			cancel()
			ticker.Stop()
			return
		case <-ticker.C:
			upToDate, err := pd.provider.IsUpToDate(ctx)
			if err != nil {
				log.Errorf("Cache processing of %v configuration provider failed: %v", pd.provider, err)
				continue
			}

			if upToDate {
				log.Debugf("No modifications in the templates stored in %v configuration provider", pd.provider)
				continue
			}

			pd.pollOnce(ctx, ac)
		}
	}
}

func (pd *configPoller) pollOnce(ctx context.Context, ac *AutoConfig) {
	// retrieve the list of newly added configurations as well
	// as removed configurations
	newConfigs, removedConfigs := pd.collect(ctx)
	if len(newConfigs) > 0 || len(removedConfigs) > 0 {
		log.Infof("%v provider: collected %d new configurations, removed %d", pd.provider, len(newConfigs), len(removedConfigs))
	} else {
		log.Debugf("%v provider: no configuration change", pd.provider)
	}

	// Process removed configs first to handle the case where a
	// container churn would result in the same configuration hash.
	ac.processRemovedConfigs(removedConfigs)

	for _, config := range newConfigs {
		if _, ok := pd.provider.(*providers.FileConfigProvider); ok {
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

		config.Provider = pd.provider.String()
		changes := ac.processNewConfig(config)
		ac.applyChanges(changes)
	}

}

// collect is just a convenient wrapper to fetch configurations from a provider and
// see what changed from the last time we called Collect().
func (pd *configPoller) collect(ctx context.Context) ([]integration.Config, []integration.Config) {
	start := time.Now()
	defer func() {
		telemetry.PollDuration.Observe(time.Since(start).Seconds(), pd.provider.String())
	}()

	fetched, err := pd.provider.Collect(ctx)
	if err != nil {
		log.Errorf("Unable to collect configurations from provider %s: %s", pd.provider, err)
		return nil, nil
	}

	return pd.storeAndDiffConfigs(fetched)
}

func (pd *configPoller) storeAndDiffConfigs(configs []integration.Config) ([]integration.Config, []integration.Config) {
	pd.configsMu.Lock()
	defer pd.configsMu.Unlock()

	var newConf []integration.Config
	var removedConf []integration.Config

	// We allocate a new map. We could do without it with a bit more processing
	// but it allows to free some memory if number of collected configs varies a lot
	fetchedMap := make(map[uint64]integration.Config, len(configs))
	for _, c := range configs {
		cHash := c.FastDigest()
		fetchedMap[cHash] = c
		if _, found := pd.configs[cHash]; found {
			delete(pd.configs, cHash)
		} else {
			newConf = append(newConf, c)
		}
	}

	for _, c := range pd.configs {
		removedConf = append(removedConf, c)
	}
	pd.configs = fetchedMap

	return newConf, removedConf
}

func (pd *configPoller) getConfigs() []integration.Config {
	pd.configsMu.Lock()
	defer pd.configsMu.Unlock()

	configs := make([]integration.Config, 0, len(pd.configs))

	for _, cfg := range pd.configs {
		configs = append(configs, cfg)
	}

	return configs
}
