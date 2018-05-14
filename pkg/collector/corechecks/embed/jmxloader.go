// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build jmx

package embed

import (
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/integration"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	log "github.com/cihub/seelog"
	yaml "gopkg.in/yaml.v2"
)

// JMXConfigCache contains the last version of jmx configs that will be given
// to jmxfetch when it calls the IPC server
var JMXConfigCache = cache.NewBasicCache()

// AddJMXCachedConfig adds a config to the jmx config cache
func AddJMXCachedConfig(config integration.Config) {
	mapConfig := map[string]interface{}{}
	mapConfig["name"] = config.Name
	mapConfig["timestamp"] = time.Now().Unix()
	mapConfig["config"] = config

	JMXConfigCache.Add(config.Name, mapConfig)
}

// JMXCheckLoader is a specific loader for checks living in this package
type JMXCheckLoader struct {
	checks []string
}

// NewJMXCheckLoader creates a loader for go checks
func NewJMXCheckLoader() (*JMXCheckLoader, error) {
	return &JMXCheckLoader{checks: []string{}}, nil
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
		if err = jmxLauncher.Configure(instance, config.InitConfig); err != nil {
			log.Errorf("jmx.loader: could not configure check: %s", err)
			return checks, err
		}
	}

	jmxLauncher.checks[fmt.Sprintf("%s.yaml", config.Name)] = struct{}{} // exists
	checks = append(checks, &jmxLauncher)

	AddJMXCachedConfig(config)
	return checks, err
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
