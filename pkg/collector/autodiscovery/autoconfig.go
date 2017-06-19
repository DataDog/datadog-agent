package autodiscovery

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/providers"
	log "github.com/cihub/seelog"
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
	loaders           []check.Loader
	configsPollTicker *time.Ticker
	stop              chan bool
	m                 sync.RWMutex
}

// NewAutoConfig creates an AutoConfig instance and start the goroutine
// responsible to poll the different configuration providers.
func NewAutoConfig(collector *collector.Collector) *AutoConfig {
	ac := &AutoConfig{
		collector:         collector,
		providers:         make([]*providerDescriptor, 0, 5),
		loaders:           make([]check.Loader, 0, 5),
		configsPollTicker: time.NewTicker(configsPollIntl),
		stop:              make(chan bool),
	}

	ac.pollConfigs()

	return ac
}

// Stop just shuts down AutoConfig in a clean way.
// AutoConfig is not supposed to be restarted, so this is expected
// to be called only once at program exit.
func (ac *AutoConfig) Stop() {
	ac.stop <- true
	ac.collector.Stop()
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
			log.Warnf("Provider %s was already added, skipping...", provider)
			return
		}
	}

	pd := &providerDescriptor{
		provider: provider,
		configs:  []check.Config{},
		poll:     shouldPoll,
	}
	ac.providers = append(ac.providers, pd)

	// call Collect() now, so providers that don't need polling will be called at least once.
	configs, _ := ac.collect(pd)
	for _, config := range configs {
		// load the check instances and schedule them
		for _, check := range ac.loadChecks(config) {
			err := ac.collector.RunCheck(check)
			if err != nil {
				log.Errorf("Unable to run Check %s: %v", check, err)
			}
		}
	}
}

// AddLoader adds a new Loader that AutoConfig can use to load a check.
func (ac *AutoConfig) AddLoader(loader check.Loader) {
	for _, l := range ac.loaders {
		if l == loader {
			log.Warnf("Loader %s was already added, skipping...", loader)
			return
		}
	}

	ac.loaders = append(ac.loaders, loader)
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

					_, _ = ac.collect(pd)

					// TODO tell the collector to stop/start/restart the corresponding checks
				}
				ac.m.RUnlock()
			}
		}
	}()
}

// collect is just a convenient wrapper to fetch configurations from a provider and
// see what changed from the last time we called Collect().
func (ac *AutoConfig) collect(pd *providerDescriptor) (new, removed []check.Config) {
	new = []check.Config{}
	removed = []check.Config{}

	fetched, err := pd.provider.Collect()
	if err != nil {
		log.Errorf("Unable to collect configurations from provider %s: %s", pd.provider, err)
		return
	}

	for _, c := range fetched {
		if !pd.contains(&c) {
			new = append(new, c)
		}
	}

	old := pd.configs
	pd.configs = fetched

	for _, c := range old {
		if !pd.contains(&c) {
			removed = append(removed, c)
		}
	}

	log.Infof("%v: collected %d new configurations, removed %d", pd.provider, len(new), len(removed))

	return
}

func (ac *AutoConfig) loadChecks(config check.Config) []check.Check {
	for _, loader := range ac.loaders {
		res, err := loader.Load(config)
		if err == nil {
			log.Infof("%v: successfully loaded check '%s'", loader, config.Name)
			return res
		}

		log.Debugf("%v: unable to load the check '%s': %s", loader, config.Name, err)
	}

	log.Errorf("Unable to load the check '%s', see debug logs for more details.", config.Name)
	return []check.Check{}
}

// check if the descriptor contains the Config passed
func (pd *providerDescriptor) contains(c *check.Config) bool {
	for _, config := range pd.configs {
		if config.Equal(c) {
			return true
		}
	}

	return false
}
