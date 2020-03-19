// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build jmx

package jmx

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// JMXCheckLoader is a specific loader for checks living in this package
type JMXCheckLoader struct {
}

// NewJMXCheckLoader creates a loader for go checks
func NewJMXCheckLoader() (*JMXCheckLoader, error) {
	state.runner.initRunner()
	return &JMXCheckLoader{}, nil
}

func splitConfig(config integration.Config, instances []integration.Data) []integration.Config {
	configs := []integration.Config{}

	for _, instance := range instances {
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
	checks := []check.Check{}

	instancesJMX := []integration.Data{}
	for _, instance := range config.Instances {
		if check.IsJMXInstance(config.Name, instance, config.InitConfig) {
			instancesJMX = append(instancesJMX, instance)
		}
	}

	for _, instance := range instancesJMX {
		if err := state.runner.configureRunner(instance, config.InitConfig); err != nil {
			log.Errorf("jmx.loader: could not configure check: %s", err)
			return checks, err
		}
	}

	for _, cf := range splitConfig(config, instancesJMX) {
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
