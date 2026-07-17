// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package collector provides the implementation of the collector
package collector

import (
	"context"
	"expvar"
	"fmt"
	"hash/fnv"
	"slices"
	"strings"
	"sync"

	yaml "go.yaml.in/yaml/v2"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	collectorcomp "github.com/DataDog/datadog-agent/comp/collector/collector/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	filter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/checkloadfailure"
	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	collectoraggregator "github.com/DataDog/datadog-agent/pkg/collector/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	corecheckLoader "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback/lookbacksender"
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
}

func init() {
	schedulerErrs = expvar.NewMap("CheckScheduler")
	schedulerErrs.Set("LoaderErrors", expvar.Func(func() interface{} {
		return errorStats.getLoaderErrors()
	}))
	schedulerErrs.Set("RunErrors", expvar.Func(func() interface{} {
		return errorStats.getRunErrors()
	}))
}

// CheckScheduler is the check scheduler
type CheckScheduler struct {
	configToChecks      map[string][]checkid.ID // cache the ID of checks we load for each config
	loaders             []check.Loader
	collector           option.Option[collectorcomp.Component]
	senderManager       sender.SenderManager
	shadowSenderManager sender.SenderManager
	shadowSenderContext context.Context
	shadowSenderCancel  context.CancelFunc
	shadowCoreLoader    check.Loader
	infraTagger         *infratags.Tagger // nil = no infra mode tagging
	healthPlatform      option.Option[healthplatformdef.Component]
	m                   sync.RWMutex
}

// InitCheckScheduler creates and returns a check scheduler
func InitCheckScheduler(collector option.Option[collectorcomp.Component], senderManager sender.SenderManager, logReceiver option.Option[integrations.Component], tagger tagger.Component, filterStore filter.Component, healthPlatform option.Option[healthplatformdef.Component]) *CheckScheduler {
	checkScheduler = &CheckScheduler{
		collector:      collector,
		senderManager:  senderManager,
		configToChecks: make(map[string][]checkid.ID),
		loaders:        make([]check.Loader, 0, len(loaders.LoaderCatalog(senderManager, logReceiver, tagger, filterStore))),
		infraTagger:    infratags.NewTagger(setup.Datadog()),
		healthPlatform: healthPlatform,
	}
	// add the check loaders
	for _, loader := range loaders.LoaderCatalog(senderManager, logReceiver, tagger, filterStore) {
		checkScheduler.addLoader(loader)
		log.Debugf("Added %s to Check Scheduler", loader)
	}

	return checkScheduler
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
	defer s.m.Unlock()

	for _, config := range configs {
		if !config.IsCheckConfig() {
			// skip non check
			continue
		}
		// unschedule all the possible checks corresponding to this config
		digest := config.Digest()
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

// Stop releases scheduler-owned resources.
func (s *CheckScheduler) Stop() {
	s.m.Lock()
	defer s.m.Unlock()

	if s.shadowSenderCancel != nil {
		s.shadowSenderCancel()
		s.shadowSenderCancel = nil
		s.shadowSenderContext = nil
	}
}

// addLoader adds a new Loader that AutoConfig can use to load a check.
func (s *CheckScheduler) addLoader(loader check.Loader) {
	if slices.Contains(s.loaders, loader) {
		log.Warnf("Loader %s was already added, skipping...", loader)
		return
	}
	s.loaders = append(s.loaders, loader)
}

// getChecks takes a check configuration and returns a slice of Check instances
// along with any error it might happen during the process
func (s *CheckScheduler) getChecks(config integration.Config, includeShadowChecks bool) ([]check.Check, error) {
	checks := []check.Check{}
	numLoaders := len(s.loaders)
	var shadowCandidates map[int]metriclookback.ShadowCandidate
	if includeShadowChecks {
		shadowCandidates = shadowCandidatesByInstance(config)
	}

	initConfig := commonInitConfig{}
	err := yaml.Unmarshal(config.InitConfig, &initConfig)
	if err != nil {
		return nil, err
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

		if len(result.loaderErrors) == numLoaders {
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
			s.reportCheckLoadFailure(config.Name, concatErr.String())
		} else {
			errorStats.removeLoaderErrors(config.Name)
			s.resolveCheckLoadFailure(config.Name)
		}
	}

	return checks, nil
}

// checkLoadFailureIssueID derives the health-issue id for a check-load
// failure on the given config name, scoped to hp's issue discriminator (the
// agent's DaemonSet uid when resolvable, so identical failures across a
// cluster collapse into one issue).
func checkLoadFailureIssueID(hp healthplatformdef.Component, configName string) string {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s\x00%s", hp.IssueDiscriminator(""), configName)
	return fmt.Sprintf("%s:%016x", checkloadfailure.IssueID, h.Sum64())
}

// reportCheckLoadFailure reports a health-platform issue for a check that
// failed to load through every configured loader. No-op when the health
// platform is unavailable or the feature is disabled.
func (s *CheckScheduler) reportCheckLoadFailure(configName, errMsg string) {
	hp, ok := s.healthPlatform.Get()
	if !ok || !setup.Datadog().GetBool("health_platform.check_load_failure.enabled") {
		return
	}
	issue, err := checkloadfailure.NewCheckLoadFailureIssue().BuildIssue(map[string]string{
		"check_name": configName,
		"errors":     errMsg,
	})
	if err != nil {
		log.Debugf("Unable to build check-load-failure issue for '%s': %v", configName, err)
		return
	}
	issue.Id = checkLoadFailureIssueID(hp, configName)
	if err := hp.ReportIssue(issue); err != nil {
		log.Debugf("Unable to report check-load-failure issue for '%s': %v", configName, err)
	}
}

// resolveCheckLoadFailure clears a previously reported check-load-failure
// issue for configName. No-op when the health platform is unavailable or no
// such issue is active.
//
// Because the issue id is scoped by IssueDiscriminator (see
// comp/healthplatform/README.md, "Cluster-wide issue collapse") rather than
// hostname, the first agent in the DaemonSet to recover clears the issue for
// every other node still affected — correct for a shared fix, but it can
// flap if only some agents recover.
func (s *CheckScheduler) resolveCheckLoadFailure(configName string) {
	hp, ok := s.healthPlatform.Get()
	if !ok {
		return
	}
	hp.ResolveIssue(checkLoadFailureIssueID(hp, configName))
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

func shadowCandidatesByInstance(config integration.Config) map[int]metriclookback.ShadowCandidate {
	candidates := metriclookback.SelectShadowCandidates([]integration.Config{config}, metriclookback.ShadowPolicyOptionsFromConfig(setup.Datadog()))
	if len(candidates) == 0 {
		return nil
	}
	byInstance := make(map[int]metriclookback.ShadowCandidate, len(candidates))
	for _, candidate := range candidates {
		byInstance[candidate.InstanceIndex] = candidate
	}
	return byInstance
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

func (s *CheckScheduler) ensureShadowSenderContext() context.Context {
	if s.shadowSenderContext == nil {
		s.shadowSenderContext, s.shadowSenderCancel = context.WithCancel(context.Background())
	}
	return s.shadowSenderContext
}

func (s *CheckScheduler) loadShadowCheck(candidate metriclookback.ShadowCandidate, loader check.Loader, sourceCheckID checkid.ID) (check.Check, error) {
	shadowSenderManager := s.shadowSenderManager
	if shadowSenderManager == nil {
		shadowSenderManager = lookbacksender.NewSenderManager(s.ensureShadowSenderContext(), "", nil, nil)
		s.shadowSenderManager = shadowSenderManager
	}
	shadowCheckID := check.ShadowID(sourceCheckID)
	checkSenderManager := &shadowCheckSenderManager{
		SenderManager: shadowSenderManager,
		shadowCheckID: shadowCheckID,
	}
	loadedCheck, err := loader.Load(checkSenderManager, candidate.SourceConfig, candidate.Instance, candidate.InstanceIndex)
	if err != nil {
		checkSenderManager.DestroySender(shadowCheckID)
		return nil, err
	}
	if !checkSenderManager.RegisterCallbackID(loadedCheck.ID()) {
		log.Warnf("Unable to register metric lookback rtloader callback route for shadow check %s loaded as %s", shadowCheckID, loadedCheck.ID())
	}
	s.applyInfraTagger(checkSenderManager, candidate.SourceConfig.Name, shadowCheckID)
	return check.NewShadowCheckForSource(loadedCheck, sourceCheckID, candidate.ShadowInterval, checkSenderManager), nil
}

type shadowCheckSenderManager struct {
	sender.SenderManager
	shadowCheckID       checkid.ID
	unregisterCallbacks []func()
}

func (m shadowCheckSenderManager) GetSender(checkid.ID) (sender.Sender, error) {
	return m.SenderManager.GetSender(m.shadowCheckID)
}

func (m shadowCheckSenderManager) SetSender(s sender.Sender, _ checkid.ID) error {
	return m.SenderManager.SetSender(s, m.shadowCheckID)
}

func (m *shadowCheckSenderManager) DestroySender(checkid.ID) {
	for _, unregister := range m.unregisterCallbacks {
		unregister()
	}
	m.unregisterCallbacks = nil
	m.SenderManager.DestroySender(m.shadowCheckID)
}

func (m *shadowCheckSenderManager) RegisterCallbackID(id checkid.ID) bool {
	unregister, ok := collectoraggregator.RegisterCheckSenderManager(id, m)
	if !ok {
		return false
	}
	m.unregisterCallbacks = append(m.unregisterCallbacks, unregister)
	return true
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
		checks, err := s.getChecks(config, populateCache)
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
	}

	return allChecks
}

// GetLoaderErrors returns the check loader errors
func GetLoaderErrors() map[string]map[string]string {
	return errorStats.getLoaderErrors()
}
