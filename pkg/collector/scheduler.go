// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package collector provides the implementation of the collector
package collector

import (
	"expvar"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp"
	"slices"
	"strings"
	"sync"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
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
	collector      option.Option[collector.Component]
	senderManager  sender.SenderManager
	m              sync.RWMutex
}

// InitCheckScheduler creates and returns a check scheduler
func InitCheckScheduler(collector option.Option[collector.Component], senderManager sender.SenderManager, logReceiver option.Option[integrations.Component], tagger tagger.Component) *CheckScheduler {
	fmt.Println("CALL INIT CHECK SCHEDULER")
	checkScheduler = &CheckScheduler{
		collector:      collector,
		senderManager:  senderManager,
		configToChecks: make(map[string][]checkid.ID),
		loaders:        make([]check.Loader, 0, len(loaders.LoaderCatalog(senderManager, logReceiver, tagger))),
	}
	// add the check loaders
	fmt.Println("CALLING LOADERS CATALOG")
	for _, loader := range loaders.LoaderCatalog(senderManager, logReceiver, tagger) {
		fmt.Printf("Adding %s to Check Scheduler\n", loader)
		checkScheduler.addLoader(loader)
		fmt.Printf("Added %s to Check Scheduler\n", loader)
	}
	return checkScheduler
}

// Schedule schedules configs to checks
func (s *CheckScheduler) Schedule(configs []integration.Config) {
	fmt.Println("SCHEDULER")
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
func (s *CheckScheduler) getChecks(config integration.Config) ([]check.Check, error) {
	fmt.Println("GET CHECKS")

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
			fmt.Printf("skip loading jmx check '%s', it is handled elsewhere\n", config.Name)
			continue
		}

		errors := []string{}
		selectedInstanceLoader := selectedLoader
		instanceConfig := commonInstanceConfig{}

		err := yaml.Unmarshal(instance, &instanceConfig)
		if err != nil {
			fmt.Printf("Unable to parse instance config for check `%s`: %v\n", config.Name, instance)
			continue
		}

		if instanceConfig.LoaderName != "" {
			selectedInstanceLoader = instanceConfig.LoaderName
		}
		if selectedInstanceLoader != "" {
			fmt.Printf("Loading check instance for check '%s' using loader %s (init_config loader: %s, instance loader: %s)\n", config.Name, selectedInstanceLoader, initConfig.LoaderName, instanceConfig.LoaderName)
		} else {
			fmt.Printf("Loading check instance for check '%s' using default loaders\n", config.Name)
		}

		fmt.Println("LEN OF LOADERS")
		fmt.Println(len(s.loaders))
		var pythonLoader check.Loader
		for _, loader := range s.loaders {
			if loader.Name() == "python" {
				pythonLoader = loader
			}

			// the loader is skipped if the loader name is set and does not match
			fmt.Printf("SELECTED INSTANCE LOADER %s VS LOADER.NAME %s\n", selectedInstanceLoader, loader.Name())
			if (selectedInstanceLoader != "") && (selectedInstanceLoader != loader.Name()) {
				fmt.Printf("Loader name %v does not match, skip loader %v for check %v\n", selectedInstanceLoader, loader.Name(), config.Name)
				continue
			}

			fmt.Println("=====================")
			fmt.Println("=====================")
			fmt.Println("=====================")
			fmt.Println(config)
			fmt.Println("=====================")
			fmt.Println("CONFIG NAME")
			fmt.Println(config.Name)
			fmt.Println("=====================")
			fmt.Println("CONFIG INSTANCE")
			fmt.Println(instance)
			fmt.Println("=====================")
			c, err := loader.Load(s.senderManager, config, instance)
			fmt.Println("CONFIG C STRING")
			fmt.Println(c)
			fmt.Println("=====================")
			fmt.Printf("%#v\n", c)
			fmt.Println("=====================")
			fmt.Println("CONFIC C STRING")
			if c != nil {
				fmt.Println(c.String())
			} else {
				fmt.Println("c is nil")
			}
			fmt.Println("=====================")
			fmt.Println("SELECTED LOADER")
			fmt.Println(selectedInstanceLoader)
			fmt.Println("=====================")
			fmt.Println("=====================")
			fmt.Println("=====================")

			if selectedInstanceLoader == "core" && c != nil && c.String() == "snmp" {
				if pythonLoader != nil {
					fmt.Println("ENTERING IN THE IF CONDITION TO SET PYTHON CHECK")
					pythonC, err := pythonLoader.Load(s.senderManager, config, instance)
					fmt.Println("PYTHON CHECK STRING IN IF CONDITION")
					fmt.Println(pythonC)
					fmt.Println("PYTHON CHECK IN IF CONDITION")
					fmt.Printf("%#v\n", pythonC)
					fmt.Println("PYTHON CHECK ERROR IN IF CONDITION")
					fmt.Println(err)
					c.(*snmp.Check).PythonCheck = pythonC
				} else {
					fmt.Println("PYTHON LOADER IS NIL IN IF CONDITION")
				}
			}

			if err != nil {
				fmt.Printf("Unable to load check '%s' using loader %s: %v\n", config.Name, loader.Name(), err)
			}
			if err == nil {
				fmt.Printf("%v: successfully loaded check '%s'\n", loader, config.Name)
				errorStats.removeLoaderErrors(config.Name)
				checks = append(checks, c)
				break
			}
			errorStats.setLoaderError(config.Name, fmt.Sprintf("%v", loader), err.Error())
			errors = append(errors, fmt.Sprintf("%v: %s", loader, err))
		}

		if len(errors) == numLoaders {
			fmt.Printf("Unable to load a check from instance of config '%s': %s\n", config.Name, strings.Join(errors, "; "))
		}
	}

	fmt.Println("LEN DE CHECK")
	fmt.Println(len(checks))

	return checks, nil
}

// GetChecksByNameForConfigs returns checks matching name for passed in configs
func GetChecksByNameForConfigs(checkName string, configs []integration.Config) []check.Check {
	fmt.Println("GET CHECKS BY NAME FOR CONFIGS")
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
