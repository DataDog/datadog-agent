// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build jmx

package jmx

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	yaml "gopkg.in/yaml.v2"
)

// JMXCheckLoader is a specific loader for checks living in this package
type JMXCheckLoader struct {
}

// NewJMXCheckLoader creates a loader for go checks
func NewJMXCheckLoader() (*JMXCheckLoader, error) {
	state.runner.initRunner()
	return &JMXCheckLoader{}, nil
}

func splitConfig(config integration.Config) []integration.Config {
	configs := []integration.Config{}

	for _, instance := range config.Instances {
		c := integration.Config{
			ADIdentifiers: config.ADIdentifiers,
			InitConfig:    config.InitConfig,
			Instances:     []integration.Data{instance},
			LogsConfig:    config.LogsConfig,
			MetricConfig:  config.MetricConfig,
			Name:          config.Name,
			Provider:      config.Provider,
		}
		configs = append(configs, c)
	}
	return configs
}

// Load returns an (empty?) list of checks and nil if it all works out
func (jl *JMXCheckLoader) Load(config integration.Config) ([]check.Check, error) {
	var err error
	checks := []check.Check{}

	if !check.IsJMXConfig(config.Name, config.InitConfig) {
		return checks, errors.New("check is not a jmx check, or unable to determine if it's so")
	}

	rawInitConfig := integration.RawMap{}
	err = yaml.Unmarshal(config.InitConfig, &rawInitConfig)
	if err != nil {
		log.Errorf("jmx.loader: could not unmarshal instance config: %s", err)
		return checks, err
	}

	for _, instance := range config.Instances {
		if err = state.runner.configureRunner(instance, config.InitConfig); err != nil {
			log.Errorf("jmx.loader: could not configure check: %s", err)
			return checks, err
		}
	}

	for _, cf := range splitConfig(config) {
		c := newJMXCheck(cf, config.Source)
		checks = append(checks, c)
	}

	return checks, nil
}

func (jl *JMXCheckLoader) String() string {
	return "JMX Check Loader"
}

func init() {
	factory := func() (check.Loader, error) {
		return NewJMXCheckLoader()
	}

	loaders.RegisterLoader(30, factory)
}
