// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package collector

import (
	"expvar"
	"fmt"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	configToChecks map[string][]check.ID // cache the ID of checks we load for each config
	loaders        []check.Loader
	collector      *Collector
	m              sync.RWMutex
}

// InitCheckScheduler creates and returns a check scheduler
func InitCheckScheduler(collector *Collector) *CheckScheduler {
	checkScheduler = &CheckScheduler{
		collector:      collector,
		configToChecks: make(map[string][]check.ID),
		loaders:        make([]check.Loader, 0, len(loaders.LoaderCatalog())),
	}
	// add the check loaders
	for _, loader := range loaders.LoaderCatalog() {
		checkScheduler.AddLoader(loader)
		log.Debugf("Added %s to Check Scheduler", loader)
	}
	return checkScheduler
}

// Schedule schedules configs to checks
func (s *CheckScheduler) Schedule(configs []integration.Config) {
	checks := s.GetChecksFromConfigs(configs, true)
	for _, c := range checks {
		_, err := s.collector.RunCheck(c)
		if err != nil {
			log.Errorf("Unable to run Check %s: %v", c, err)
			errorStats.setRunError(c.ID(), err.Error())
			continue
		}
	}
}

// Unschedule unschedules checks matching configs
func (s *CheckScheduler) Unschedule(configs []integration.Config) {
	for _, config := range configs {
		if !config.IsCheckConfig() {
			// skip non check configs.
			continue
		}
		// unschedule all the possible checks corresponding to this config
		digest := config.Digest()
		ids := s.configToChecks[digest]
		stopped := map[check.ID]struct{}{}
		for _, id := range ids {
			// `StopCheck` might time out so we don't risk to block
			// the polling loop forever
			err := s.collector.StopCheck(id)
			if err != nil {
				log.Errorf("Error stopping check %s: %s", id, err)
				errorStats.setRunError(id, err.Error())
			} else {
				stopped[id] = struct{}{}
			}
		}

		// remove the entry from `configToChecks`
		if len(stopped) == len(s.configToChecks[digest]) {
			// we managed to stop all the checks for this config
			delete(s.configToChecks, digest)
		} else {
			// keep the checks we failed to stop in `configToChecks`
			dangling := []check.ID{}
			for _, id := range s.configToChecks[digest] {
				if _, found := stopped[id]; !found {
					dangling = append(dangling, id)
				}
			}
			s.configToChecks[digest] = dangling
		}
	}
}

// Stop handles clean stop of registered schedulers
func (s *CheckScheduler) Stop() {
	if s.collector != nil {
		s.collector.Stop()
	}
}

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
	for _, loader := range s.loaders {
		res, err := loader.Load(config)
		if err == nil {
			log.Debugf("%v: successfully loaded check '%s'", loader, config.Name)
			errorStats.removeLoaderErrors(config.Name)
			return res, nil
		}
		// Check if some check instances were loaded correctly (can occur if there's multiple check instances)
		if len(res) != 0 {
			return res, nil
		}
		errorStats.setLoaderError(config.Name, fmt.Sprintf("%v", loader), err.Error())
		log.Debugf("%v: unable to load the check '%s': %s", loader, config.Name, err)
	}

	return []check.Check{}, fmt.Errorf("unable to load any check from config '%s'", config.Name)
}

// GetChecksByNameForConfigs returns checks matching name for passed in configs
func GetChecksByNameForConfigs(checkName string, configs []integration.Config) []check.Check {
	var checks []check.Check
	if checkScheduler == nil {
		return checks
	}
	// try to also match `FooCheck` if `foo` was passed
	titleCheck := fmt.Sprintf("%s%s", strings.Title(checkName), "Check")

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
