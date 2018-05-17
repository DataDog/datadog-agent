// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build jmx

package jmx

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

type jmxState struct {
	configs     *cache.BasicCache
	runnerError chan struct{}
	lock        *sync.Mutex
}

var state jmxState = jmxState{
	configs:     cache.NewBasicCache(),
	runnerError: make(chan struct{}),
	lock:        &sync.Mutex{},
}

func (s *jmxState) scheduleCheck(c *JMXCheck) error {
	s.lock.Lock()
	if runner == nil {
		err := check.Retry(5*time.Second, 3, startRunner, "jmxfetch")
		if err != nil {
			return err
		}
	}
	s.configs.Add(string(c.id), c.config)

	s.lock.Unlock()
	return nil
}

func (s *jmxState) unscheduleCheck(c *JMXCheck) {
	s.lock.Lock()
	s.configs.Remove(string(c.id))
	s.lock.Unlock()
}

func AddScheduledConfig(c integration.Config) {
	state.configs.Add(fmt.Sprintf("%v_%v", c.Name, c.Digest()), c)
}

func GetScheduledConfigs() map[string]integration.Config {
	configs := make(map[string]integration.Config, len(state.configs.Items()))
	for name, config := range state.configs.Items() {
		configs[name] = config.(integration.Config)
	}
	return configs
}

func GetScheduledConfigsModificationTimestamp() int64 {
	return state.configs.GetModified()
}
