// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package collector provides the implementation of the collector
package collector

import (
	"expvar"
	"fmt"
	"sync"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	collectorcomp "github.com/DataDog/datadog-agent/comp/collector/collector/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	filter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

var (
	schedulerErrs  *expvar.Map
	errorStats     = newCollectorErrors()
	checkScheduler *CheckScheduler
)

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
	loader         *CheckLoader
	collector      option.Option[collectorcomp.Component]
	m              sync.RWMutex
}

// InitCheckScheduler creates and returns a check scheduler.
// The checkLoader must be created via NewCheckLoader; passing the same instance
// to collectorimpl (as an optional fx dep) allows collectorimpl.start() to call
// ConfigureShadow on it before any checks are scheduled.
func InitCheckScheduler(collector option.Option[collectorcomp.Component], checkLoader *CheckLoader) *CheckScheduler {
	checkScheduler = &CheckScheduler{
		collector:      collector,
		loader:         checkLoader,
		configToChecks: make(map[string][]checkid.ID),
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

	// Shadow scheduling: load separate check instances with the shadow SenderManager
	// and enter them into the shadow scheduler at the configured higher frequency.
	if s.loader.shadowScheduler != nil {
		for _, config := range configs {
			if !config.IsCheckConfig() {
				continue
			}
			if !s.loader.isShadowed(config.Name) {
				continue
			}
			shadowChecks, err := s.loader.LoadChecks(config, s.loader.shadowSenderMgr)
			if err != nil {
				log.Errorf("Shadow pipeline: unable to load checks for '%s': %v", config.Name, err)
				continue
			}
			for _, c := range shadowChecks {
				sc := newShadowCheck(c)
				if err := s.loader.shadowScheduler.EnterWithInterval(sc, s.loader.shadowInterval); err != nil {
					log.Errorf("Shadow pipeline: unable to schedule check '%s': %v", sc.ID(), err)
				}
			}
		}
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

			// Cancel the corresponding shadow check from the shadow scheduler.
			if s.loader.shadowScheduler != nil {
				shadowID := checkid.ID(string(id) + ":shadow")
				if err := s.loader.shadowScheduler.Cancel(shadowID); err != nil {
					log.Errorf("Shadow pipeline: error cancelling check %s: %s", shadowID, err)
				}
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
		if config.HasFilter(filter.MetricsFilter) {
			log.Debugf("Config %s is filtered out for metrics collection, ignoring it", config.Name)
			continue
		}
		configDigest := config.Digest()
		checks, err := s.loader.LoadChecks(config, s.loader.senderManager)
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

// GetCheckScheduler returns the global CheckScheduler instance created by InitCheckScheduler.
// Returns nil if InitCheckScheduler has not been called yet.
func GetCheckScheduler() *CheckScheduler {
	return checkScheduler
}

// GetLoaderErrors returns the check loader errors
func GetLoaderErrors() map[string]map[string]string {
	return errorStats.getLoaderErrors()
}
