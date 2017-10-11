// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build jmx

package embed

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	log "github.com/cihub/seelog"
	yaml "gopkg.in/yaml.v2"
)

const (
	windowsToken       = '\\'
	unixToken          = '/'
	autoDiscoveryToken = "#### AUTO-DISCOVERY ####\n"
)

var JMXConfigCache = cache.NewBasicCache()

// JMXCheckLoader is a specific loader for checks living in this package
type JMXCheckLoader struct {
	checks []string
}

// NewJMXCheckLoader creates a loader for go checks
func NewJMXCheckLoader() (*JMXCheckLoader, error) {
	basePath := config.Datadog.GetString("jmx_pipe_path")
	pipeName := config.Datadog.GetString("jmx_pipe_name")

	var sep byte
	var pipePath string
	if strings.Contains(basePath, string(windowsToken)) {
		sep = byte(windowsToken)
	} else {
		sep = byte(unixToken)
	}

	if basePath[len(basePath)-1] == sep {
		pipePath = fmt.Sprintf("%s%s", basePath, pipeName)
	} else {
		pipePath = fmt.Sprintf("%s%c%s", basePath, sep, pipeName)
	}

	pipe, err := util.GetPipe(pipePath)
	if err != nil {
		log.Errorf("Error getting pipe: %v", err)
		return nil, errors.New("unable to initialize pipe")
	}

	if err := pipe.Open(); err != nil {
		log.Errorf("Error opening pipe: %v", err)
		return nil, errors.New("unable to initialize pipe")
	}

	return &JMXCheckLoader{checks: []string{}}, nil
}

// Load returns an (empty?) list of checks and nil if it all works out
func (jl *JMXCheckLoader) Load(config check.Config) ([]check.Check, error) {
	var err error
	checks := []check.Check{}
	mapConfig := map[string]interface{}{}

	if !check.IsConfigJMX(config.Name, config.InitConfig) {
		return checks, errors.New("check is not a jmx check, or unable to determine if it's so")
	}

	rawInitConfig := check.ConfigRawMap{}
	err = yaml.Unmarshal(config.InitConfig, &rawInitConfig)
	if err != nil {
		log.Errorf("jmx.loader: could not unmarshal instance config: %s", err)
		return checks, err
	}
	mapConfig["name"] = config.Name
	mapConfig["timestamp"] = time.Now().Unix()

	for _, instance := range config.Instances {
		if err = jmxLauncher.Configure(instance, config.InitConfig); err != nil {
			log.Errorf("jmx.loader: could not configure check: %s", err)
			return checks, err
		}
	}

	JMXConfigCache.Add(config.Name, mapConfig)
	jmxLauncher.checks[fmt.Sprintf("%s.yaml", config.Name)] = struct{}{} // exists
	checks = append(checks, &jmxLauncher)
	mapConfig["config"] = config

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
