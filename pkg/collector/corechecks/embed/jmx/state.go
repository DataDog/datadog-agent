// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jmx

package jmx

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
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

func (s *jmxState) scheduleCheck(c *JMXCheck) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	if !s.runner.started {
		err := check.Retry(5*time.Second, 3, s.runner.startRunner, "jmxfetch")
		if err != nil {
			return err
		}
	}
	s.configs.Add(string(c.id), c.config)
	return nil
}

func (s *jmxState) unscheduleCheck(c *JMXCheck) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.configs.Remove(string(c.id))
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
