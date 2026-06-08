// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
	"fmt"
	"slices"
	"strings"

	yaml "go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type commonInitConfig struct {
	LoaderName string `yaml:"loader"`
}

type commonInstanceConfig struct {
	LoaderName string `yaml:"loader"`
}

// CheckLoader manages check loaders and knows how to instantiate check instances
// from integration configs. It is decoupled from routing concerns — which
// SenderManager the resulting checks use is passed at load time via LoadChecks,
// allowing the same loader to serve both the normal pipeline and an alternate
// destination (e.g. the shadow pipeline) without duplication.
type CheckLoader struct {
	loaders       []check.Loader
	senderManager sender.SenderManager
}

// newCheckLoader creates a CheckLoader populated with all registered loaders.
func newCheckLoader(senderManager sender.SenderManager, logReceiver option.Option[integrations.Component], taggerComp tagger.Component, filterStore workloadfilter.Component) *CheckLoader {
	cl := &CheckLoader{
		senderManager: senderManager,
		loaders:       make([]check.Loader, 0),
	}
	for _, loader := range loaders.LoaderCatalog(senderManager, logReceiver, taggerComp, filterStore) {
		cl.addLoader(loader)
		log.Debugf("Added %s to Check Loader", loader)
	}
	return cl
}

// addLoader adds a Loader, skipping duplicates.
func (cl *CheckLoader) addLoader(loader check.Loader) {
	if slices.Contains(cl.loaders, loader) {
		log.Warnf("Loader %s was already added, skipping...", loader)
		return
	}
	cl.loaders = append(cl.loaders, loader)
}

// LoadChecks instantiates check instances for a single config using the given
// SenderManager. Passing a different SenderManager (e.g. for the shadow pipeline)
// allows routing samples to an alternate destination without touching the config.
func (cl *CheckLoader) LoadChecks(config integration.Config, sm sender.SenderManager) ([]check.Check, error) {
	checks := []check.Check{}
	numLoaders := len(cl.loaders)

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

		loaderErrors := make(map[string]error, len(cl.loaders))
		for _, loader := range cl.loaders {
			if (selectedInstanceLoader != "") && (selectedInstanceLoader != loader.Name()) {
				log.Debugf("Loader name %v does not match, skip loader %v for check %v", selectedInstanceLoader, loader.Name(), config.Name)
				continue
			}
			c, err := loader.Load(sm, config, instance, instanceIndex)
			if err == nil {
				log.Debugf("%v: successfully loaded check '%s'", loader, config.Name)
				checks = append(checks, c)
				break
			}
			loaderErrors[fmt.Sprintf("%v", loader)] = err
		}

		if len(loaderErrors) == numLoaders {
			var concatErr strings.Builder
			for loaderName, err := range loaderErrors {
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

	return checks, nil
}
