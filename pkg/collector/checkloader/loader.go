// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package checkloader contains shared check loading behavior used by collector
// schedulers.
package checkloader

import (
	"fmt"
	"strings"

	yaml "go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type loaderConfig struct {
	LoaderName string `yaml:"loader"`
}

// LoaderErrorRecorder records loader errors for status/debug output.
type LoaderErrorRecorder interface {
	SetLoaderError(checkName, loaderName, err string)
	RemoveLoaderErrors(checkName string)
}

// Loader applies the normal collector loader selection rules to check configs.
type Loader struct {
	loaders       []check.Loader
	senderManager sender.SenderManager
	errorRecorder LoaderErrorRecorder
}

// New creates a shared check loader.
func New(loaders []check.Loader, senderManager sender.SenderManager, errorRecorder LoaderErrorRecorder) *Loader {
	if errorRecorder == nil {
		errorRecorder = noopLoaderErrorRecorder{}
	}
	return &Loader{
		loaders:       loaders,
		senderManager: senderManager,
		errorRecorder: errorRecorder,
	}
}

// LoadConfig loads all check instances from the provided config.
func (l *Loader) LoadConfig(config integration.Config) ([]check.Check, error) {
	checks := []check.Check{}

	initLoader, err := InitConfigLoader(config.InitConfig)
	if err != nil {
		return nil, err
	}

	for instanceIndex, instance := range config.Instances {
		c, loaded, err := l.loadInstance(l.senderManager, config, initLoader, instance, instanceIndex)
		if err != nil {
			return nil, err
		}
		if loaded {
			checks = append(checks, c)
		}
	}

	return checks, nil
}

// LoadInstance loads one check instance from the provided config with the
// provided sender manager.
func (l *Loader) LoadInstance(senderManager sender.SenderManager, config integration.Config, instance integration.Data, instanceIndex int) (check.Check, bool, error) {
	initLoader, err := InitConfigLoader(config.InitConfig)
	if err != nil {
		return nil, false, err
	}

	return l.loadInstance(senderManager, config, initLoader, instance, instanceIndex)
}

// InitConfigLoader returns the loader selected by init_config.
func InitConfigLoader(initConfigData integration.Data) (string, error) {
	var cfg loaderConfig
	if err := yaml.Unmarshal(initConfigData, &cfg); err != nil {
		return "", err
	}
	return cfg.LoaderName, nil
}

// SelectedInstanceLoader returns the loader selected for an instance after
// applying the normal instance-level override rule.
func SelectedInstanceLoader(initLoader string, instance integration.Data) (string, error) {
	selectedLoader := initLoader
	var cfg loaderConfig
	if err := yaml.Unmarshal(instance, &cfg); err != nil {
		return "", err
	}
	if cfg.LoaderName != "" {
		selectedLoader = cfg.LoaderName
	}
	return selectedLoader, nil
}

func (l *Loader) loadInstance(senderManager sender.SenderManager, config integration.Config, initLoader string, instance integration.Data, instanceIndex int) (check.Check, bool, error) {
	if check.IsJMXInstance(config.Name, instance, config.InitConfig) {
		log.Debugf("skip loading jmx check '%s', it is handled elsewhere", config.Name)
		return nil, false, nil
	}

	selectedInstanceLoader, err := SelectedInstanceLoader(initLoader, instance)
	if err != nil {
		log.Warnf("Unable to parse instance config for check `%s`: %v", config.Name, instance)
		return nil, false, nil
	}

	instanceLoader := ""
	if selectedInstanceLoader != initLoader {
		instanceLoader = selectedInstanceLoader
	}
	if selectedInstanceLoader != "" {
		log.Debugf("Loading check instance for check '%s' using loader %s (init_config loader: %s, instance loader: %s)", config.Name, selectedInstanceLoader, initLoader, instanceLoader)
	} else {
		log.Debugf("Loading check instance for check '%s' using default loaders", config.Name)
	}

	loaderErrors := make(map[string]error, len(l.loaders))
	for _, loader := range l.loaders {
		if selectedInstanceLoader != "" && selectedInstanceLoader != loader.Name() {
			log.Debugf("Loader name %v does not match, skip loader %v for check %v", selectedInstanceLoader, loader.Name(), config.Name)
			continue
		}
		c, err := loader.Load(senderManager, config, instance, instanceIndex)
		if err == nil {
			log.Debugf("%v: successfully loaded check '%s'", loader, config.Name)
			l.removeLoaderErrors(config.Name)
			return c, true, nil
		}
		loaderErrors[fmt.Sprintf("%v", loader)] = err
	}

	if len(loaderErrors) == len(l.loaders) {
		var concatErr strings.Builder
		for loaderName, err := range loaderErrors {
			errMsg := err.Error()
			l.setLoaderError(config.Name, loaderName, errMsg)

			concatErr.WriteString(loaderName)
			concatErr.WriteString(": ")
			concatErr.WriteString(errMsg)
			concatErr.WriteString("; ")
		}
		log.Errorf("Unable to load a check from instance of config '%s': %s", config.Name, concatErr.String())
	} else {
		l.removeLoaderErrors(config.Name)
	}

	return nil, false, nil
}

func (l *Loader) setLoaderError(checkName, loaderName, err string) {
	l.errorRecorder.SetLoaderError(checkName, loaderName, err)
}

func (l *Loader) removeLoaderErrors(checkName string) {
	l.errorRecorder.RemoveLoaderErrors(checkName)
}

type noopLoaderErrorRecorder struct{}

func (noopLoaderErrorRecorder) SetLoaderError(string, string, string) {}
func (noopLoaderErrorRecorder) RemoveLoaderErrors(string)             {}
