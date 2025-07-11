// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package autodiscoveryimpl implements the agent's autodiscovery mechanism.
package autodiscoveryimpl

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"go.uber.org/fx"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers"
	providerTypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	autodiscoveryStatus "github.com/DataDog/datadog-agent/comp/core/autodiscovery/status"
	acTelemetry "github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	logComp "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/status"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

var listenerCandidateIntl = 30 * time.Second

// dependencies is the set of dependencies for the AutoConfig component.
type dependencies struct {
	fx.In
	Lc          fx.Lifecycle
	Config      configComponent.Component
	Log         logComp.Component
	TaggerComp  tagger.Component
	Secrets     secrets.Component
	WMeta       option.Option[workloadmeta.Component]
	FilterStore workloadfilter.Component
	Telemetry   telemetry.Component
}

// AutoConfig implements the agent's autodiscovery mechanism.  It is
// responsible to collect integrations configurations from different sources
// and then "schedule" or "unschedule" them by notifying subscribers.  See the
// module README for details.
type AutoConfig struct {
	configPollers            []*configPoller
	listeners                []listeners.ServiceListener
	listenerCandidates       map[string]*listenerCandidate
	listenerRetryStop        chan struct{}
	schedulerController      *scheduler.Controller
	listenerStop             chan struct{}
	healthListening          *health.Handle
	newService               chan listeners.Service
	delService               chan listeners.Service
	store                    *store
	cfgMgr                   configManager
	serviceListenerFactories map[string]listeners.ServiceListenerFactory
	providerCatalog          map[string]providerTypes.ConfigProviderFactory
	wmeta                    option.Option[workloadmeta.Component]
	taggerComp               tagger.Component
	logs                     logComp.Component
	filterStore              workloadfilter.Component
	telemetryStore           *acTelemetry.Store

	// m covers the `configPollers`, `listenerCandidates`, `listeners`, and `listenerRetryStop`, but
	// not the values they point to.
	m sync.RWMutex
}

const (
	// wmetaCheckInitialInterval is the initial interval to check if wmeta is ready
	wmetaCheckInitialInterval = 20 * time.Millisecond
	// wmetaCheckMaxInterval is the maximum interval to check if wmeta is ready
	wmetaCheckMaxInterval = 1 * time.Second
	// wmetaCheckMaxElapsedTime is the maximum time to wait for wmeta being ready
	wmetaCheckMaxElapsedTime = 10 * time.Second
)

type provides struct {
	fx.Out

	Comp           autodiscovery.Component
	StatusProvider status.InformationProvider
	Endpoint       api.AgentEndpointProvider
	FlareProvider  flaretypes.Provider
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(
			newProvides,
		))
}

func newProvides(deps dependencies) provides {
	c := newAutoConfig(deps)
	return provides{
		Comp:           c,
		StatusProvider: status.NewInformationProvider(autodiscoveryStatus.GetProvider(c, deps.FilterStore)),

		Endpoint:      api.NewAgentEndpointProvider(c.(*AutoConfig).writeConfigCheck, "/config-check", "GET"),
		FlareProvider: flaretypes.NewProvider(c.(*AutoConfig).fillFlare),
	}
}

var _ autodiscovery.Component = (*AutoConfig)(nil)

type listenerCandidate struct {
	factory listeners.ServiceListenerFactory
	options listeners.ServiceListernerDeps
}

func (l *listenerCandidate) try() (listeners.ServiceListener, error) {
	return l.factory(l.options)
}

// newAutoConfig creates an AutoConfig instance and starts it.
func newAutoConfig(deps dependencies) autodiscovery.Component {
	schController := scheduler.NewController()
	// Non-blocking start of the scheduler controller
	go func() {
		retries := 0
		expBackoff := backoff.NewExponentialBackOff()
		expBackoff.InitialInterval = wmetaCheckInitialInterval
		expBackoff.MaxInterval = wmetaCheckMaxInterval
		expBackoff.MaxElapsedTime = wmetaCheckMaxElapsedTime
		err := backoff.Retry(func() error {
			instance, found := deps.WMeta.Get()
			if found {
				if instance.IsInitialized() {
					deps.Log.Infof("Workloadmeta collectors are ready, starting autodiscovery scheduler controller")
					schController.Start()
					return nil
				}
				retries++
				deps.Log.Debugf("Workloadmeta collectors are not ready, will possibly retry")
				return errors.New("workloadmeta not initialized")
			}
			schController.Start()
			return nil
		}, expBackoff)
		if err != nil {
			deps.Log.Errorf("Workloadmeta collectors are not ready after %d retries: %s, starting check scheduler controller anyway.", retries, err)
			schController.Start()
		}
	}()

	ac := createNewAutoConfig(schController, deps.Secrets, deps.WMeta, deps.TaggerComp, deps.Log, deps.Telemetry, deps.FilterStore)
	deps.Lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			ac.start()
			return nil
		},
		OnStop: func(_ context.Context) error {
			ac.stop()
			return nil
		},
	})
	return ac
}

// createNewAutoConfig creates an AutoConfig instance (without starting).
func createNewAutoConfig(schedulerController *scheduler.Controller, secretResolver secrets.Component, wmeta option.Option[workloadmeta.Component], taggerComp tagger.Component, logs logComp.Component, telemetryComp telemetry.Component, filterStore workloadfilter.Component) *AutoConfig {
	cfgMgr := newReconcilingConfigManager(secretResolver)
	ac := &AutoConfig{
		configPollers:            make([]*configPoller, 0, 9),
		listenerCandidates:       make(map[string]*listenerCandidate),
		listenerRetryStop:        nil, // We'll open it if needed
		listenerStop:             make(chan struct{}),
		healthListening:          health.RegisterLiveness("ad-servicelistening"),
		newService:               make(chan listeners.Service),
		delService:               make(chan listeners.Service),
		store:                    newStore(),
		cfgMgr:                   cfgMgr,
		schedulerController:      schedulerController,
		serviceListenerFactories: make(map[string]listeners.ServiceListenerFactory),
		providerCatalog:          make(map[string]providerTypes.ConfigProviderFactory),
		wmeta:                    wmeta,
		taggerComp:               taggerComp,
		logs:                     logs,
		filterStore:              filterStore,
		telemetryStore:           acTelemetry.NewStore(telemetryComp),
	}
	return ac
}

// serviceListening is the main management goroutine for services.
// It waits for service events to trigger template resolution and
// checks the tags on existing services are up to date.
func (ac *AutoConfig) serviceListening() {
	for {
		select {
		case <-ac.listenerStop:
			ac.healthListening.Deregister() //nolint:errcheck
			return
		case <-ac.healthListening.C: // To be considered healthy
		case svc := <-ac.newService:
			ac.processNewService(svc)
		case svc := <-ac.delService:
			ac.processDelService(svc)
		}
	}
}

func (ac *AutoConfig) writeConfigCheck(w http.ResponseWriter, r *http.Request) {
	raw := r != nil && r.URL.Query().Get("raw") == "true"

	var configCheckResponse integration.ConfigCheckResponse
	if raw {
		configCheckResponse = ac.getRawConfigCheck()
	} else {
		configCheckResponse = ac.GetConfigCheck()
	}

	jsonConfig, err := json.Marshal(configCheckResponse)
	if err != nil {
		httputils.SetJSONError(w, err, 500)
		return
	}

	w.Write(jsonConfig)
}

// GetConfigCheck returns scrubbed information from all configuration providers
func (ac *AutoConfig) GetConfigCheck() integration.ConfigCheckResponse {
	return ac.buildConfigCheckResponse(true)
}

// getRawConfigCheck returns information from all configuration providers
func (ac *AutoConfig) getRawConfigCheck() integration.ConfigCheckResponse {
	return ac.buildConfigCheckResponse(false)
}

// buildConfigCheckResponse is a helper to build the ConfigCheckResponse for both scrubbed and raw configs.
func (ac *AutoConfig) buildConfigCheckResponse(scrub bool) integration.ConfigCheckResponse {
	var response integration.ConfigCheckResponse

	configSlice := ac.GetAllConfigs()
	sort.Slice(configSlice, func(i, j int) bool {
		return configSlice[i].Name < configSlice[j].Name
	})

	configResponses := make([]integration.ConfigResponse, len(configSlice))

	for i, config := range configSlice {
		instanceIDs := make([]string, len(config.Instances))
		for j, instance := range config.Instances {
			instanceIDs[j] = string(checkid.BuildID(config.Name, config.FastDigest(), instance, config.InitConfig))
		}

		if scrub {
			config = ac.scrubConfig(config)
		}

		configResponses[i] = integration.ConfigResponse{
			Config:      config,
			InstanceIDs: instanceIDs,
		}
	}

	response.Configs = configResponses
	response.ResolveWarnings = GetResolveWarnings()
	response.ConfigErrors = GetConfigErrors()

	if scrub {
		unresolved := ac.getUnresolvedConfigs()
		scrubbedUnresolved := make(map[string]integration.Config, len(unresolved))
		for id, config := range unresolved {
			scrubbedUnresolved[id] = ac.scrubConfig(config)
		}
		response.Unresolved = scrubbedUnresolved
	} else {
		response.Unresolved = ac.getUnresolvedConfigs()
	}

	return response
}

func (ac *AutoConfig) scrubConfig(config integration.Config) integration.Config {
	scrubbedConfig := config
	scrubbedInstances := make([]integration.Data, len(config.Instances))
	for instanceIndex, inst := range config.Instances {
		scrubbedData, err := scrubData(inst)
		if err != nil {
			ac.logs.Warnf("error scrubbing secrets from config: %s", err)
			continue
		}
		scrubbedInstances[instanceIndex] = scrubbedData
	}
	scrubbedConfig.Instances = scrubbedInstances

	if len(config.InitConfig) > 0 {
		scrubbedData, err := scrubData(config.InitConfig)
		if err != nil {
			ac.logs.Warnf("error scrubbing secrets from init config: %s", err)
			scrubbedConfig.InitConfig = []byte{}
		} else {
			scrubbedConfig.InitConfig = scrubbedData
		}
	}

	if len(config.MetricConfig) > 0 {
		scrubbedData, err := scrubData(config.MetricConfig)
		if err != nil {
			ac.logs.Warnf("error scrubbing secrets from metric config: %s", err)
			scrubbedConfig.MetricConfig = []byte{}
		} else {
			scrubbedConfig.MetricConfig = scrubbedData
		}
	}

	if len(config.LogsConfig) > 0 {
		scrubbedData, err := scrubData(config.LogsConfig)
		if err != nil {
			ac.logs.Warnf("error scrubbing secrets from logs config: %s", err)
			scrubbedConfig.LogsConfig = []byte{}
		} else {
			scrubbedConfig.LogsConfig = scrubbedData
		}
	}

	return scrubbedConfig
}

func scrubData(data []byte) ([]byte, error) {
	return scrubber.ScrubYaml(data)
}

// fillFlare add the config-checks log to flares.
func (ac *AutoConfig) fillFlare(fb flaretypes.FlareBuilder) error {
	fb.AddFileFromFunc("config-check.log", func() ([]byte, error) { //nolint:errcheck
		var b bytes.Buffer

		writer := bufio.NewWriter(&b)
		response := ac.GetConfigCheck()
		flare.PrintConfigCheck(writer, response, true)
		writer.Flush()

		return b.Bytes(), nil
	})
	return nil
}

// start will listen to the service channels before anything is sent to them
func (ac *AutoConfig) start() {
	listeners.RegisterListeners(ac.serviceListenerFactories)
	providers.RegisterProviders(ac.providerCatalog)
	setupAcErrors()
	// Start the service listener
	go ac.serviceListening()
}

// stop just shuts down AutoConfig in a clean way.
// AutoConfig is not supposed to be restarted, so this is expected
// to be called only once at program exit.
func (ac *AutoConfig) stop() {
	// stop polled config providers without holding ac.m
	for _, pd := range ac.getConfigPollers() {
		pd.stop()
	}

	// stop the service listener
	ac.listenerStop <- struct{}{}

	// stop the meta scheduler
	ac.schedulerController.Stop()

	ac.m.RLock()
	defer ac.m.RUnlock()

	// stop the listener retry logic if running
	if ac.listenerRetryStop != nil {
		ac.listenerRetryStop <- struct{}{}
	}

	// stop all the listeners
	for _, l := range ac.listeners {
		l.Stop()
	}
}

// AddConfigProvider adds a new configuration provider to AutoConfig.
// Callers must pass a flag to indicate whether the configuration provider
// expects to be polled and at which interval or it's fine for it to be invoked only once in the
// Agent lifetime.
// If the config provider is polled, the routine is scheduled right away
func (ac *AutoConfig) AddConfigProvider(provider providerTypes.ConfigProvider, shouldPoll bool, pollInterval time.Duration) {
	if shouldPoll && pollInterval <= 0 {
		log.Warnf("Polling interval <= 0 for AD provider: %s, deactivating polling", provider.String())
		shouldPoll = false
	}
	cp := newConfigPoller(provider, shouldPoll, pollInterval, ac.telemetryStore)

	ac.m.Lock()
	defer ac.m.Unlock()
	ac.configPollers = append(ac.configPollers, cp)
}

// LoadAndRun loads all of the integration configs it can find
// and schedules them. Should always be run once so providers
// that don't need polling will be queried at least once
func (ac *AutoConfig) LoadAndRun(ctx context.Context) {
	for _, cp := range ac.getConfigPollers() {
		cp.start(ctx, ac)
		if cp.canPoll {
			log.Infof("Started config provider %q, polled every %s", cp.provider.String(), cp.pollInterval.String())
		} else {
			log.Infof("Started config provider %q", cp.provider.String())
		}

		// TODO: this probably belongs somewhere inside the file config
		// provider itself, but since it already lived in AD it's been
		// moved here for the moment.
		if fileConfPd, ok := cp.provider.(*providers.FileConfigProvider); ok {
			// Grab any errors that occurred when reading the YAML file
			for name, e := range fileConfPd.Errors {
				errorStats.setConfigError(name, e)
			}
		}
	}
}

// GetAllConfigs returns all resolved and non-template configs known to
// AutoConfig.
func (ac *AutoConfig) GetAllConfigs() []integration.Config {
	var configs []integration.Config

	ac.cfgMgr.mapOverLoadedConfigs(func(scheduledConfigs map[string]integration.Config) {
		configs = make([]integration.Config, 0, len(scheduledConfigs))
		for _, config := range scheduledConfigs {
			configs = append(configs, config)
		}
	})

	return configs
}

// GetTelemetryStore returns autodiscovery telemetry store.
func (ac *AutoConfig) GetTelemetryStore() *acTelemetry.Store {
	return ac.telemetryStore
}

// processNewConfig store (in template cache) and resolves a given config,
// returning the changes to be made.
func (ac *AutoConfig) processNewConfig(config integration.Config) integration.ConfigChanges {
	// add default metrics to collect to JMX checks
	if check.CollectDefaultMetrics(config) {
		metrics := ac.store.getJMXMetricsForConfigName(config.Name)
		if len(metrics) == 0 {
			log.Infof("%s doesn't have an additional metric configuration file: not collecting default metrics", config.Name)
		} else if err := config.AddMetrics(metrics); err != nil {
			log.Infof("Unable to add default metrics to collect to %s check: %s", config.Name, err)
		}
	}

	changes, changedIDsOfSecretsWithConfigs := ac.cfgMgr.processNewConfig(config)
	ac.store.setIDsOfChecksWithSecrets(changedIDsOfSecretsWithConfigs)
	return changes
}

// AddListeners tries to initialise the listeners listed in the given configs. A first
// try is done synchronously. If a listener fails with a ErrWillRetry, the initialization
// will be re-triggered later until success or ErrPermaFail.
func (ac *AutoConfig) AddListeners(listenerConfigs []pkgconfigsetup.Listeners) {
	ac.addListenerCandidates(listenerConfigs)
	remaining := ac.initListenerCandidates()
	if !remaining {
		return
	}

	// Start the retry logic if we have remaining candidates and it is not already running
	ac.m.Lock()
	defer ac.m.Unlock()
	if ac.listenerRetryStop == nil {
		ac.listenerRetryStop = make(chan struct{})
		go ac.retryListenerCandidates()
	}
}

func (ac *AutoConfig) addListenerCandidates(listenerConfigs []pkgconfigsetup.Listeners) {
	ac.m.Lock()
	defer ac.m.Unlock()

	for _, c := range listenerConfigs {
		factory, ok := ac.serviceListenerFactories[c.Name]
		if !ok {
			// Factory has not been registered.
			log.Warnf("Listener %s was not registered", c.Name)
			continue
		}
		log.Debugf("Listener %s was registered", c.Name)
		factoryOptions := listeners.ServiceListernerDeps{
			Config:    &c,
			Telemetry: ac.telemetryStore,
			Filter:    ac.filterStore,
			Tagger:    ac.taggerComp,
			Wmeta:     ac.wmeta,
		}

		ac.listenerCandidates[c.Name] = &listenerCandidate{factory: factory, options: factoryOptions}
	}
}

func (ac *AutoConfig) initListenerCandidates() bool {
	ac.m.Lock()
	defer ac.m.Unlock()

	for name, candidate := range ac.listenerCandidates {
		listener, err := candidate.try()
		switch {
		case err == nil:
			// Init successful, let's start listening
			log.Infof("%s listener successfully started", name)
			ac.listeners = append(ac.listeners, listener)
			listener.Listen(ac.newService, ac.delService)
			delete(ac.listenerCandidates, name)
		case retry.IsErrWillRetry(err):
			// Log an info and keep in candidates
			log.Infof("%s listener cannot start, will retry: %s", name, err)
		default:
			// Log an error and remove from candidates
			log.Errorf("%s listener cannot start: %s", name, err)
			delete(ac.listenerCandidates, name)
		}
	}

	return len(ac.listenerCandidates) > 0
}

func (ac *AutoConfig) retryListenerCandidates() {
	retryTicker := time.NewTicker(listenerCandidateIntl)
	defer func() {
		// Stop ticker
		retryTicker.Stop()
		// Cleanup channel before exiting so that we can re-start the routine later
		ac.m.Lock()
		defer ac.m.Unlock()
		close(ac.listenerRetryStop)
		ac.listenerRetryStop = nil
	}()

	for {
		select {
		case <-ac.listenerRetryStop:
			return
		case <-retryTicker.C:
			remaining := ac.initListenerCandidates()
			if !remaining {
				return
			}
		}
	}
}

// AddScheduler a new scheduler to receive configurations.
//
// Previously scheduled configurations that have not subsequently been
// unscheduled can be replayed with the replayConfigs flag.  This replay occurs
// immediately, before the AddScheduler call returns.
func (ac *AutoConfig) AddScheduler(name string, s scheduler.Scheduler, replayConfigs bool) {
	ac.schedulerController.Register(name, s, replayConfigs)
}

// RemoveScheduler allows to remove a scheduler from the AD system.
func (ac *AutoConfig) RemoveScheduler(name string) {
	ac.schedulerController.Deregister(name)
}

func (ac *AutoConfig) processRemovedConfigs(configs []integration.Config) {
	changes := ac.cfgMgr.processDelConfigs(configs)
	ac.applyChanges(changes)
	ac.deleteMappingsOfCheckIDsWithSecrets(changes.Unschedule)
}

// getUnresolvedConfigs returns all the active configs, in their unresolved
// state.
func (ac *AutoConfig) getUnresolvedConfigs() map[string]integration.Config {
	return ac.cfgMgr.getActiveConfigs()
}

// GetIDOfCheckWithEncryptedSecrets returns the ID that a checkID had before
// decrypting its secrets.
// Returns empty if the check with the given ID does not have any secrets.
func (ac *AutoConfig) GetIDOfCheckWithEncryptedSecrets(checkID checkid.ID) checkid.ID {
	return ac.store.getIDOfCheckWithEncryptedSecrets(checkID)
}

// GetProviderCatalog returns all registered ConfigProviderFactory.
func (ac *AutoConfig) GetProviderCatalog() map[string]providerTypes.ConfigProviderFactory {
	return ac.providerCatalog
}

// processNewService takes a service, tries to match it against templates and
// triggers scheduling events if it finds a valid config for it.
func (ac *AutoConfig) processNewService(svc listeners.Service) {
	changes := ac.cfgMgr.processNewService(svc)
	ac.applyChanges(changes)
}

// processDelService takes a service, stops its associated checks, and updates the cache
func (ac *AutoConfig) processDelService(svc listeners.Service) {
	changes := ac.cfgMgr.processDelService(svc)
	ac.applyChanges(changes)
}

// GetAutodiscoveryErrors fetches AD errors from each ConfigProvider.  The
// resulting data structure maps provider name to resource name to a set of
// unique error messages.  The resource names do not match other identifiers
// and are only intended for display in diagnostic tools like `agent status`.
func (ac *AutoConfig) GetAutodiscoveryErrors() map[string]map[string]providerTypes.ErrorMsgSet {
	errors := map[string]map[string]providerTypes.ErrorMsgSet{}
	for _, cp := range ac.getConfigPollers() {
		configErrors := cp.provider.GetConfigErrors()
		if len(configErrors) > 0 {
			errors[cp.provider.String()] = configErrors
		}
	}
	return errors
}

// applyChanges applies a configChanges object. This always unschedules first.
func (ac *AutoConfig) applyChanges(changes integration.ConfigChanges) {
	telemetryStorePresent := ac.telemetryStore != nil

	if len(changes.Unschedule) > 0 {
		for _, conf := range changes.Unschedule {
			log.Tracef("Unscheduling %s\n", conf.Dump(false))
			if telemetryStorePresent {
				ac.telemetryStore.ScheduledConfigs.Dec(conf.Provider, configType(conf))
			}
		}
	}

	if len(changes.Schedule) > 0 {
		for _, conf := range changes.Schedule {
			log.Tracef("Scheduling %s\n", conf.Dump(false))
			if telemetryStorePresent {
				ac.telemetryStore.ScheduledConfigs.Inc(conf.Provider, configType(conf))
			}
		}
	}
	ac.schedulerController.ApplyChanges(changes)
}

func (ac *AutoConfig) deleteMappingsOfCheckIDsWithSecrets(configs []integration.Config) {
	var checkIDsToDelete []checkid.ID
	for _, configToDelete := range configs {
		for _, instance := range configToDelete.Instances {
			checkID := checkid.BuildID(configToDelete.Name, configToDelete.FastDigest(), instance, configToDelete.InitConfig)
			checkIDsToDelete = append(checkIDsToDelete, checkID)
		}
	}

	ac.store.deleteMappingsOfCheckIDsWithSecrets(checkIDsToDelete)
}

// getConfigPollers gets a slice of config pollers that can be used without holding
// ac.m.
func (ac *AutoConfig) getConfigPollers() []*configPoller {
	ac.m.RLock()
	defer ac.m.RUnlock()

	// this value is only ever appended to, so the sliced elements will not change and
	// no race can occur.
	return ac.configPollers
}

func configType(c integration.Config) string {
	if c.IsLogConfig() {
		return "logs"
	}

	if c.IsCheckConfig() {
		return "check"
	}

	if c.ClusterCheck {
		return "clustercheck"
	}

	return "unknown"
}
