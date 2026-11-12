// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package collector provides the implementation of the collector
package collector

import (
	"errors"
	"expvar"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	yaml "go.yaml.in/yaml/v3"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	collectorcomp "github.com/DataDog/datadog-agent/comp/collector/collector/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	filter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	corecheckLoader "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/infratags"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

var (
	schedulerErrs  *expvar.Map
	errorStats     = newCollectorErrors()
	checkScheduler *CheckScheduler
)

type commonInitConfig struct {
	LoaderName string `yaml:"loader"`
}

type commonInstanceConfig struct {
	LoaderName string `yaml:"loader"`
}

type loadInstanceResult struct {
	check        check.Check
	loader       check.Loader
	loaderErrors map[string]error
	depsPending  []string
}

func init() {
	schedulerErrs = expvar.NewMap("CheckScheduler")
	schedulerErrs.Set("LoaderErrors", expvar.Func(func() interface{} {
		return errorStats.getLoaderErrors()
	}))
	schedulerErrs.Set("RunErrors", expvar.Func(func() interface{} {
		return errorStats.getRunErrors()
	}))
	schedulerErrs.Set("DeferredChecks", expvar.Func(func() interface{} {
		if checkScheduler == nil {
			return []map[string]interface{}{}
		}
		return checkScheduler.deferredStatus()
	}))
}

// CheckScheduler is the check scheduler
type CheckScheduler struct {
	configToChecks      map[string][]checkid.ID // cache the ID of checks we load for each config
	deferredConfigs     map[string]deferredConfig
	configDeps          map[string][]string
	depInstaller        DepInstaller
	depInstallInFlight  bool
	depInstallMu        sync.Mutex
	deferredStop        chan struct{}
	deferredWg          sync.WaitGroup
	loaders             []check.Loader
	collector           option.Option[collectorcomp.Component]
	senderManager       sender.SenderManager
	shadowSenderManager sender.SenderManager
	shadowCoreLoader    check.Loader
	infraTagger         *infratags.MetricTagger // nil = no infra mode tagging
	m                   sync.RWMutex
}

// InitCheckScheduler creates and returns a check scheduler
func InitCheckScheduler(collector option.Option[collectorcomp.Component], senderManager sender.SenderManager, logReceiver option.Option[integrations.Component], tagger tagger.Component, filterStore filter.Component) *CheckScheduler {
	checkScheduler = &CheckScheduler{
		collector:       collector,
		senderManager:   senderManager,
		configToChecks:  make(map[string][]checkid.ID),
		deferredConfigs: make(map[string]deferredConfig),
		configDeps:      make(map[string][]string),
		loaders:         make([]check.Loader, 0, len(loaders.LoaderCatalog(senderManager, logReceiver, tagger, filterStore))),
		infraTagger:     infratags.NewMetricTagger(setup.Datadog()),
	}
	// add the check loaders
	for _, loader := range loaders.LoaderCatalog(senderManager, logReceiver, tagger, filterStore) {
		checkScheduler.addLoader(loader)
		log.Debugf("Added %s to Check Scheduler", loader)
	}

	checkScheduler.depInstaller = newDepInstallerClient()
	log.Infof("On-demand python dependencies: deferred checks will be retried every %s", deferredRetryInterval())
	checkScheduler.startDeferredLoop()

	return checkScheduler
}

// SetMetricLookbackShadowSenderManager sets the sender manager used by metric
// lookback shadow checks loaded by this scheduler.
func (s *CheckScheduler) SetMetricLookbackShadowSenderManager(senderManager sender.SenderManager) {
	if s == nil {
		return
	}
	s.m.Lock()
	defer s.m.Unlock()
	s.shadowSenderManager = senderManager
}

// Schedule schedules configs to checks
func (s *CheckScheduler) Schedule(configs []integration.Config) {
	if coll, ok := s.collector.Get(); ok {
		checks := s.GetChecksFromConfigs(configs, true)
		for _, c := range checks {
			// Check if this check is allowed in infra basic mode
			if !IsCheckAllowed(c.String(), setup.Datadog()) {
				log.Warnf("Check %s is not allowed in infrastructure mode %q, skipping", c.String(), setup.Datadog().GetString("infrastructure_mode"))
				continue
			}
			_, err := coll.RunCheck(c)
			if err != nil {
				log.Errorf("Unable to run Check %s: %v", c, err)
				errorStats.setRunError(c.ID(), err.Error())
				continue
			}
		}
	} else {
		log.Errorf("Collector not available, unable to schedule checks")
	}
}

// Unschedule unschedules checks matching configs
func (s *CheckScheduler) Unschedule(configs []integration.Config) {
	s.m.Lock()
	removedDeps := []string{}
	defer func() {
		s.removeOrphanDeps(removedDeps)
		s.m.Unlock()
	}()

	for _, config := range configs {
		if !config.IsCheckConfig() {
			// skip non check
			continue
		}
		// unschedule all the possible checks corresponding to this config
		digest := config.Digest()
		if d, ok := s.deferredConfigs[digest]; ok {
			removedDeps = append(removedDeps, d.deps...)
			delete(s.deferredConfigs, digest)
		}
		if deps, ok := s.configDeps[digest]; ok {
			removedDeps = append(removedDeps, deps...)
			delete(s.configDeps, digest)
		}
		ids := s.configToChecks[digest]
		stopped := map[checkid.ID]struct{}{}
		for _, id := range ids {
			if coll, ok := s.collector.Get(); ok {
				// `StopCheck` might time out so we don't risk to block
				// the polling loop forever
				err := coll.StopCheck(id)
				if err != nil {
					log.Errorf("Error stopping check %s: %s", id, err)
					errorStats.setRunError(id, err.Error())
				} else {
					stopped[id] = struct{}{}
				}
			} else {
				log.Errorf("Collector not available, unable to stop check %s", id)
			}
		}

		// remove the entry from `configToChecks`
		if len(stopped) == len(s.configToChecks[digest]) {
			// we managed to stop all the checks for this config
			delete(s.configToChecks, digest)
		} else {
			// keep the checks we failed to stop in `configToChecks`
			dangling := []checkid.ID{}
			for _, id := range s.configToChecks[digest] {
				if _, found := stopped[id]; !found {
					dangling = append(dangling, id)
				}
			}
			s.configToChecks[digest] = dangling
		}
	}
}

// Stop satisfies the autodiscovery scheduler interface.
func (s *CheckScheduler) Stop() {
	if s.deferredStop != nil {
		close(s.deferredStop)
		s.deferredStop = nil
	}
	s.deferredWg.Wait()
}

// addLoader adds a new Loader that AutoConfig can use to load a check.
func (s *CheckScheduler) addLoader(loader check.Loader) {
	if slices.Contains(s.loaders, loader) {
		log.Warnf("Loader %s was already added, skipping...", loader)
		return
	}
	s.loaders = append(s.loaders, loader)
}

// getChecks takes a check configuration and returns a slice of Check
// instances, the python dependencies the check is waiting on (if any), and
// any error it might happen during the process
func (s *CheckScheduler) getChecks(config integration.Config, includeShadowChecks bool) ([]check.Check, []string, error) {
	checks := []check.Check{}
	depsPending := []string{}
	numLoaders := len(s.loaders)
	var shadowCandidates map[int]shadowCandidate
	if includeShadowChecks {
		shadowCandidates = shadowCandidatesByInstance(config)
	}

	initConfig := commonInitConfig{}
	err := yaml.Unmarshal(config.InitConfig, &initConfig)
	if err != nil {
		return nil, nil, err
	}
	selectedLoader := initConfig.LoaderName

	for instanceIndex, instance := range config.Instances {
		if check.IsJMXInstance(config.Name, instance, config.InitConfig) {
			log.Debugf("skip loading jmx check '%s', it is handled elsewhere", config.Name)
			continue
		}

		selectedInstanceLoader := selectedLoader
		instanceConfig := commonInstanceConfig{}

		err := yaml.Unmarshal(instance, &instanceConfig)
		if err != nil {
			log.Warnf("Unable to parse instance config for check `%s`: %v", config.Name, instance)
			continue
		}

		if instanceConfig.LoaderName != "" {
			selectedInstanceLoader = instanceConfig.LoaderName
		}
		if selectedInstanceLoader != "" {
			log.Debugf("Loading check instance for check '%s' using loader %s (init_config loader: %s, instance loader: %s)", config.Name, selectedInstanceLoader, initConfig.LoaderName, instanceConfig.LoaderName)
		} else {
			log.Debugf("Loading check instance for check '%s' using default loaders", config.Name)
		}

		result := s.loadCheckInstance(s.senderManager, config, instance, instanceIndex, selectedInstanceLoader)

		if result.check == nil && result.depsPending != nil {
			depsPending = append(depsPending, result.depsPending...)
		}

		if result.check != nil {
			log.Debugf("%v: successfully loaded check '%s'", result.loader, config.Name)
			s.applyInfraTagger(s.senderManager, config.Name, result.check.ID())
			checks = append(checks, result.check)
			if includeShadowChecks {
				if candidate, found := shadowCandidates[instanceIndex]; found {
					sourceCheckID := result.check.ID()
					shadowLoader, ok := s.shadowLoaderFor(result.loader)
					if !ok {
						log.Debugf("Skipping metric lookback shadow check %s: loader %s does not support shadow execution", check.ShadowID(sourceCheckID), result.loader.Name())
						continue
					}
					if shadowCheck, err := s.loadShadowCheck(candidate, shadowLoader, sourceCheckID); err != nil {
						log.Warnf("Unable to load metric lookback shadow check %s: %v", check.ShadowID(sourceCheckID), err)
					} else {
						checks = append(checks, shadowCheck)
					}
				}
			}
		}

		if len(result.loaderErrors) == numLoaders && result.check == nil && result.depsPending == nil {
			var concatErr strings.Builder
			for loaderName, err := range result.loaderErrors {
				errMsg := err.Error()
				errorStats.setLoaderError(config.Name, loaderName, errMsg)

				concatErr.WriteString(loaderName)
				concatErr.WriteString(": ")
				concatErr.WriteString(errMsg)
				concatErr.WriteString("; ")
			}
			log.Errorf("Unable to load a check from instance of config '%s': %s", config.Name, concatErr.String())
		} else {
			errorStats.removeLoaderErrors(config.Name)
		}
	}

	return checks, depsPending, nil
}

func (s *CheckScheduler) shadowLoaderFor(loader check.Loader) (check.Loader, bool) {
	switch loader.Name() {
	case corecheckLoader.GoCheckLoaderName:
		if s.shadowCoreLoader != nil {
			return s.shadowCoreLoader, true
		}
		shadowLoader, err := corecheckLoader.NewGoCheckLoader(corecheckLoader.WithLoadMode(corecheckLoader.ShadowLoadMode))
		if err != nil {
			log.Debugf("Unable to create metric lookback shadow loader for %s: %v", loader.Name(), err)
			return nil, false
		}
		s.shadowCoreLoader = shadowLoader
		return shadowLoader, true
	case "python":
		return loader, true
	default:
		return nil, false
	}
}

func (s *CheckScheduler) loadCheckInstance(senderManager sender.SenderManager, config integration.Config, instance integration.Data, instanceIndex int, selectedInstanceLoader string) loadInstanceResult {
	result := loadInstanceResult{loaderErrors: make(map[string]error, len(s.loaders))}
	for _, loader := range s.loaders {
		// the loader is skipped if the loader name is set and does not match
		if (selectedInstanceLoader != "") && (selectedInstanceLoader != loader.Name()) {
			log.Debugf("Loader name %v does not match, skip loader %v for check %v", selectedInstanceLoader, loader.Name(), config.Name)
			continue
		}
		c, err := loader.Load(senderManager, config, instance, instanceIndex)
		if err == nil {
			result.check = c
			result.loader = loader
			return result
		}
		var pending *loaders.DepsPendingError
		if errors.As(err, &pending) {
			result.depsPending = pending.Deps
		}
		result.loaderErrors[fmt.Sprintf("%v", loader)] = err
	}
	return result
}

func (s *CheckScheduler) applyInfraTagger(senderManager sender.SenderManager, checkName string, checkID checkid.ID) {
	if s.infraTagger == nil || !s.infraTagger.IsCheckEligible(checkName) {
		return
	}
	chkSender, err := senderManager.GetSender(checkID)
	if err != nil {
		log.Debugf("infra mode tags: skipping %s (%s): %v", checkName, checkID, err)
		return
	}
	chkSender.SetInfraTagger(s.infraTagger)
}

// GetChecksByNameForConfigs returns checks matching name for passed in configs
func GetChecksByNameForConfigs(checkName string, configs []integration.Config) []check.Check {
	var checks []check.Check
	if checkScheduler == nil {
		return checks
	}
	// try to also match `FooCheck` if `foo` was passed
	titled := cases.Title(language.English, cases.NoLower).String(checkName)
	titleCheck := fmt.Sprintf("%s%s", titled, "Check")

	for _, c := range checkScheduler.GetChecksFromConfigs(configs, false) {
		if checkName == c.String() || titleCheck == c.String() {
			checks = append(checks, c)
		}
	}
	return checks
}

// GetChecksFromConfigs gets all the check instances for given configurations.
// When populateCache is true, the call is part of scheduling and includes
// selected metric lookback shadow checks in the scheduler cache.
func (s *CheckScheduler) GetChecksFromConfigs(configs []integration.Config, populateCache bool) []check.Check {
	s.m.Lock()
	defer s.m.Unlock()
	return s.getChecksFromConfigsLocked(configs, populateCache)
}

// getChecksFromConfigsLocked implements GetChecksFromConfigs and expects the
// scheduler mutex to be held.
func (s *CheckScheduler) getChecksFromConfigsLocked(configs []integration.Config, populateCache bool) []check.Check {
	var allChecks []check.Check
	for _, config := range configs {
		if !config.IsCheckConfig() {
			// skip non check configs.
			continue
		}
		if config.HasFilter(filter.MetricsFilter) {
			log.Debugf("Config %s is filtered out for metrics collection, ignoring it", config.Name)
			continue
		}
		configDigest := config.Digest()
		checks, depsPending, err := s.getChecks(config, populateCache)
		if err != nil {
			log.Errorf("Unable to load the check: %v", err)
			continue
		}
		for _, c := range checks {
			allChecks = append(allChecks, c)
			if populateCache {
				// store the checks we schedule for this config locally
				s.configToChecks[configDigest] = append(s.configToChecks[configDigest], c.ID())
			}
		}
		if !populateCache {
			if len(depsPending) > 0 {
				log.Warnf("Check %s cannot load yet, dependencies %v are missing; they are installed on demand when the agent is running", config.Name, depsPending)
			}
			continue
		}
		if len(checks) > 0 {
			if deps := s.declaredDeps(config); len(deps) > 0 {
				s.configDeps[configDigest] = deps
			}
			if _, deferred := s.deferredConfigs[configDigest]; deferred {
				log.Infof("Deferred check %s now loads, scheduling it", config.Name)
				delete(s.deferredConfigs, configDigest)
			}
		} else if len(depsPending) > 0 {
			d := deferredConfig{config: config, deps: depsPending, deferredAt: time.Now()}
			if existing, ok := s.deferredConfigs[configDigest]; ok {
				d.attempts = existing.attempts
				d.deferredAt = existing.deferredAt
			}
			s.deferredConfigs[configDigest] = d
			log.Debugf("Check %s deferred, waiting for dependencies %v (attempt %d)", config.Name, depsPending, d.attempts)
			s.triggerDepInstall(depsPending, d.attempts)
		}
	}

	return allChecks
}

// GetLoaderErrors returns the check loader errors
func GetLoaderErrors() map[string]map[string]string {
	return errorStats.getLoaderErrors()
}
