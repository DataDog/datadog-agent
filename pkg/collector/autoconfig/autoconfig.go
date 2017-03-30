package autoconfig

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/providers"
)

var (
	configsPollIntl = 10 * time.Second
	configPipeBuf   = 100
)

type providerDescriptor struct {
	provider providers.ConfigProvider
	configs  []check.Config
	poll     bool
}

// AutoConfig is responsible to collect checks configurations from
// different sources and then create, update or destroy check instances
// with the help of the Collector.
// It is also responsible to listen to containers related events and act
// accordingly.
type AutoConfig struct {
	collector         *collector.Collector
	providers         []*providerDescriptor
	configsPollTicker *time.Ticker
	stop              chan bool
	m                 sync.RWMutex
}

// NewAutoConfig creates an AutoConfig instance and start the goroutine
// responsible to poll the different configuration providers.
func NewAutoConfig(collector *collector.Collector) *AutoConfig {
	ac := &AutoConfig{
		collector:         collector,
		providers:         make([]*providerDescriptor, 5),
		configsPollTicker: time.NewTicker(configsPollIntl),
		stop:              make(chan bool),
	}

	ac.pollConfigs()

	return ac
}

// Stop just shuts down AutoConfig in a clean way. AutoConfig is not
// supposed to be stopped and restarted.
func (ac *AutoConfig) Stop() {
	ac.stop <- true
}

// AddProvider adds a new configuration provider to AutoConfig.
// Callers must pass a flag to indicate whether the configuration provider
// expects to be polled or it's fine for it to be invoked only once in the
// Agent lifetime.
func (ac *AutoConfig) AddProvider(provider providers.ConfigProvider, shouldPoll bool) {
	ac.m.Lock()
	defer ac.m.Unlock()

	for _, pd := range ac.providers {
		if pd.provider == provider {
			// we already know this configuration provider, don't do anything
			// TODO: log we won't add this provider
			return
		}
	}

	pd := &providerDescriptor{
		provider: provider,
		configs:  []check.Config{},
		poll:     shouldPoll,
	}
	ac.providers = append(ac.providers, pd)

	// call Collect() at least once
	ac.collect(pd)
}

// pollConfigs periodically calls Collect() on all the configuration
// providers that have been requested to be polled
func (ac *AutoConfig) pollConfigs() {
	go func() {
		for {
			select {
			case <-ac.stop:
				ac.configsPollTicker.Stop()
				return
			case <-ac.configsPollTicker.C:
				ac.m.RLock()
				// invoke Collect on the known providers
				for _, pd := range ac.providers {
					// skip providers that don't want to be polled
					if !pd.poll {
						continue
					}

					ac.collect(pd)
					// new, changed, removed := ac.collect(pd)
					// TODO tell the collector to stop/start/restart the corresponding checks
				}
				ac.m.RUnlock()
			}
		}
	}()
}

// collect is just a convenient wrapper to fetch configurations from a provider and
// see what changed from the last time we called Collect().
func (ac *AutoConfig) collect(pd *providerDescriptor) (new, changed, removed []check.Config) {
	new = []check.Config{}
	changed = []check.Config{}
	removed = []check.Config{}
	configs, err := pd.provider.Collect()
	if err != nil {
		// TODO log error
		return new, changed, removed
	}

	// TODO
	return configs, changed, removed
}
