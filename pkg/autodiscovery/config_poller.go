// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package autodiscovery

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// configPoller keeps track of the configurations loaded by a certain
// `ConfigProvider` and whether it should be polled or not.
type configPoller struct {
	provider     providers.ConfigProvider
	configs      []integration.Config
	canPoll      bool
	isPolling    bool
	pollInterval time.Duration
	stopChan     chan struct{}
	healthHandle *health.Handle
}

func newConfigPoller(provider providers.ConfigProvider, canPoll bool, interval time.Duration) *configPoller {
	return &configPoller{
		provider:     provider,
		configs:      []integration.Config{},
		canPoll:      canPoll,
		pollInterval: interval,
	}
}

// contains checks if the providerDescriptor contains the Config passed
func (pd *configPoller) contains(c *integration.Config) bool {
	for _, config := range pd.configs {
		if config.Equal(c) {
			return true
		}
	}
	return false
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
func (pd *configPoller) start(ac *AutoConfig) {
	if !pd.canPoll {
		return
	}
	pd.stopChan = make(chan struct{})
	pd.healthHandle = health.RegisterLiveness(fmt.Sprintf("ad-config-provider-%s", pd.provider.String()))
	pd.isPolling = true
	go pd.poll(ac)
}

// poll polls config of the corresponding config provider
func (pd *configPoller) poll(ac *AutoConfig) {
	ticker := time.NewTicker(pd.pollInterval)
	for {
		select {
		case <-pd.healthHandle.C:
		case <-pd.stopChan:
			pd.healthHandle.Deregister() //nolint:errcheck
			ticker.Stop()
			return
		case <-ticker.C:
			log.Tracef("Polling %s config provider", pd.provider.String())
			// Check if the CPupdate cache is up to date. Fill it and trigger a Collect() if outdated.
			upToDate, err := pd.provider.IsUpToDate()
			if err != nil {
				log.Errorf("Cache processing of %v configuration provider failed: %v", pd.provider, err)
			}
			if upToDate == true {
				log.Debugf("No modifications in the templates stored in %v configuration provider", pd.provider)
				break
			}

			// retrieve the list of newly added configurations as well
			// as removed configurations
			newConfigs, removedConfigs := pd.collect()
			if len(newConfigs) > 0 || len(removedConfigs) > 0 {
				log.Infof("%v provider: collected %d new configurations, removed %d", pd.provider, len(newConfigs), len(removedConfigs))
			} else {
				log.Debugf("%v provider: no configuration change", pd.provider)
			}
			// Process removed configs first to handle the case where a
			// container churn would result in the same configuration hash.
			ac.processRemovedConfigs(removedConfigs)
			// We can also remove any cached template
			ac.removeConfigTemplates(removedConfigs)

			for _, config := range newConfigs {
				config.Provider = pd.provider.String()
				resolvedConfigs := ac.processNewConfig(config)
				ac.schedule(resolvedConfigs)
			}
		}
	}
}

// collect is just a convenient wrapper to fetch configurations from a provider and
// see what changed from the last time we called Collect().
func (pd *configPoller) collect() ([]integration.Config, []integration.Config) {
	var newConf []integration.Config
	var removedConf []integration.Config
	old := pd.configs

	fetched, err := pd.provider.Collect()
	if err != nil {
		log.Errorf("Unable to collect configurations from provider %s: %s", pd.provider, err)
		return nil, nil
	}

	for _, c := range fetched {
		if !pd.contains(&c) {
			newConf = append(newConf, c)
		}
	}

	pd.configs = fetched
	for _, c := range old {
		if !pd.contains(&c) {
			removedConf = append(removedConf, c)
		}
	}
	return newConf, removedConf
}
