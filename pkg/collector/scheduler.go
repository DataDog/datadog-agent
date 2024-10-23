// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package collector provides the implementation of the collector
package collector

import (
	"expvar"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	yaml "gopkg.in/yaml.v2"
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
	configToChecks map[string][]checkid.ID // cache the ID of checks we load for each config
	loaders        []check.Loader
	collector      optional.Option[collector.Component]
	senderManager  sender.SenderManager
	m              sync.RWMutex
}

// InitCheckScheduler creates and returns a check scheduler
func InitCheckScheduler(collector optional.Option[collector.Component], senderManager sender.SenderManager, logReceiver optional.Option[integrations.Component], tagger tagger.Component) *CheckScheduler {
	checkScheduler = &CheckScheduler{
		collector:      collector,
		senderManager:  senderManager,
		configToChecks: make(map[string][]checkid.ID),
		loaders:        make([]check.Loader, 0, len(loaders.LoaderCatalog(senderManager, logReceiver, tagger))),
	}
	// add the check loaders
	for _, loader := range loaders.LoaderCatalog(senderManager, logReceiver, tagger) {
		checkScheduler.AddLoader(loader)
		log.Debugf("Added %s to Check Scheduler", loader)
	}
	return checkScheduler
}

// Schedule schedules configs to checks
func (s *CheckScheduler) Schedule(configs []integration.Config) {
	if coll, ok := s.collector.Get(); ok {
		checks := s.GetChecksFromConfigs(configs, true)
		for _, c := range checks {
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

// Stop is a stub to satisfy the scheduler interface
func (s *CheckScheduler) Stop() {}

// AddLoader adds a new Loader that AutoConfig can use to load a check.
func (s *CheckScheduler) AddLoader(loader check.Loader) {
	for _, l := range s.loaders {
		if l == loader {
			log.Warnf("Loader %s was already added, skipping...", loader)
			return
		}
	}
	s.loaders = append(s.loaders, loader)
}

// getChecks takes a check configuration and returns a slice of Check instances
// along with any error it might happen during the process
func (s *CheckScheduler) getChecks(config integration.Config) ([]check.Check, error) {
	checks := []check.Check{}
	numLoaders := len(s.loaders)

	initConfig := commonInitConfig{}
	err := yaml.Unmarshal(config.InitConfig, &initConfig)
	if err != nil {
		return nil, err
	}
	selectedLoader := initConfig.LoaderName

	for _, instance := range config.Instances {
		if check.IsJMXInstance(config.Name, instance, config.InitConfig) {
			log.Debugf("skip loading jmx check '%s', it is handled elsewhere", config.Name)
			continue
		}

		errors := []string{}
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

		for _, loader := range s.loaders {
			// the loader is skipped if the loader name is set and does not match
			if (selectedInstanceLoader != "") && (selectedInstanceLoader != loader.Name()) {
				log.Debugf("Loader name %v does not match, skip loader %v for check %v", selectedInstanceLoader, loader.Name(), config.Name)
				continue
			}
			c, err := loader.Load(s.senderManager, config, instance)
			if err == nil {
				log.Debugf("%v: successfully loaded check '%s'", loader, config.Name)
				errorStats.removeLoaderErrors(config.Name)
				checks = append(checks, c)
				break
			}
			errorStats.setLoaderError(config.Name, fmt.Sprintf("%v", loader), err.Error())
			errors = append(errors, fmt.Sprintf("%v: %s", loader, err))
		}

		if len(errors) == numLoaders {
			log.Errorf("Unable to load a check from instance of config '%s': %s", config.Name, strings.Join(errors, "; "))
		}
	}

	return checks, nil
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

// GetChecksFromConfigs gets all the check instances for given configurations
// optionally can populate the configToChecks cache
func (s *CheckScheduler) GetChecksFromConfigs(configs []integration.Config, populateCache bool) []check.Check {
	s.m.Lock()
	defer s.m.Unlock()

	var allChecks []check.Check
	for _, config := range configs {
		if !config.IsCheckConfig() {
			// skip non check configs.
			continue
		}
		if config.HasFilter(containers.MetricsFilter) {
			log.Debugf("Config %s is filtered out for metrics collection, ignoring it", config.Name)
			continue
		}
		configDigest := config.Digest()
		checks, err := s.getChecks(config)
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
