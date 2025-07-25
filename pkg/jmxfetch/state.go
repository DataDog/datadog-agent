// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jmx

package jmxfetch

import (
	"fmt"
	"sync"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type jmxState struct {
	configs     *cache.BasicCache
	runnerError chan struct{}
	runner      *runner
	lock        *sync.Mutex
}

var state = jmxState{
	configs:     cache.NewBasicCache(),
	runnerError: make(chan struct{}),
	runner:      &runner{},
	lock:        &sync.Mutex{},
}

func (s *jmxState) scheduleConfig(id string, config integration.Config) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	if !s.runner.started {
		err := check.Retry(5*time.Second, 3, s.runner.startRunner, "jmxfetch")
		if err != nil {
			return err
		}
	}
	s.configs.Add(id, config)
	return nil
}

func (s *jmxState) unscheduleConfig(id string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.configs.Remove(id)
}

func (s *jmxState) addScheduledConfig(c integration.Config) {
	s.configs.Add(fmt.Sprintf("%v_%v", c.Name, c.Digest()), c)
}

func (s *jmxState) getScheduledConfigs() map[string]integration.Config {
	configs := make(map[string]integration.Config, len(s.configs.Items()))
	for name, config := range s.configs.Items() {
		configs[name] = config.(integration.Config)
	}
	return configs
}

func (s *jmxState) getScheduledConfigsModificationTimestamp() int64 {
	return s.configs.GetModified()
}

// AddScheduledConfig adds a config to the list of scheduled config.
// This list is pulled by jmxfetch periodically to update its list of configs.
func AddScheduledConfig(c integration.Config) {
	state.addScheduledConfig(c)
}

// GetScheduledConfigs returns the list of scheduled jmx configs.
func GetScheduledConfigs() map[string]integration.Config {
	return state.getScheduledConfigs()
}

// GetScheduledConfigsModificationTimestamp returns the last timestamp at which
// the list of scheduled configuration got updated.
func GetScheduledConfigsModificationTimestamp() int64 {
	return state.getScheduledConfigsModificationTimestamp()
}

// StopJmxfetch stops the jmxfetch process if it is running
func StopJmxfetch() {
	err := state.runner.stopRunner()
	if err != nil {
		log.Errorf("failure to kill jmxfetch process: %s", err)
	}
}

// InitRunner inits the runner and injects the dogstatsd server component and the IPC component (used to get the auth token for the jmxfetch process).
func InitRunner(server dogstatsdServer.Component, logger jmxlogger.Component, ipc ipc.Component) {
	state.runner.initRunner(server, logger, ipc)
}

// GetIntegrations returns the JMXFetch integrations' instances as a map[string]interface{}.
func GetIntegrations() (map[string]interface{}, error) {
	integrations := map[string]interface{}{}
	configs := map[string]integration.JSONMap{}

	for name, config := range GetScheduledConfigs() {
		var rawInitConfig integration.RawMap
		err := yaml.Unmarshal(config.InitConfig, &rawInitConfig)
		if err != nil {
			return nil, fmt.Errorf("unable to parse JMX configuration: %w", err)
		}

		c := map[string]interface{}{}
		c["init_config"] = GetJSONSerializableMap(rawInitConfig)
		instances := []integration.JSONMap{}
		for _, instance := range config.Instances {
			var rawInstanceConfig integration.JSONMap
			err := yaml.Unmarshal(instance, &rawInstanceConfig)
			if err != nil {
				return nil, fmt.Errorf("unable to parse JMX configuration: %w", err)
			}
			instances = append(instances, GetJSONSerializableMap(rawInstanceConfig).(integration.JSONMap))
		}

		integration.ConfigSourceToMetadataMap(config.Source, c)
		c["instances"] = instances
		c["check_name"] = config.Name

		configs[name] = c
	}
	integrations["configs"] = configs
	integrations["timestamp"] = time.Now().Unix()

	return integrations, nil
}
