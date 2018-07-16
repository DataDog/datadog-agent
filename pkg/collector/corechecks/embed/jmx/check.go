// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build jmx

package jmx

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type JMXCheck struct {
	id     check.ID
	name   string
	config integration.Config
	stop   chan struct{}
}

func newJMXCheck(config integration.Config) *JMXCheck {
	check := &JMXCheck{
		config: config,
		stop:   make(chan struct{}),
		name:   config.Name,
		id:     check.ID(fmt.Sprintf("%v_%v", config.Name, config.Digest())),
	}
	check.Configure(config.InitConfig, config.MetricConfig)

	return check
}

func (c *JMXCheck) Run() error {
	err := state.scheduleCheck(c)
	if err != nil {
		return err
	}

	select {
	case <-state.runnerError:
		return fmt.Errorf("jmxfetch exited, stopping %s : %s", c.name, err)
	case <-c.stop:
		log.Debug("jmx check %s stopped", c.name)
	}

	return nil
}

func (c *JMXCheck) Stop() {
	close(c.stop)
	state.unscheduleCheck(c)
}

func (c *JMXCheck) String() string {
	return c.name
}

func (c *JMXCheck) Version() string {
	return ""
}

func (c *JMXCheck) Configure(config integration.Data, initConfig integration.Data) error {
	return nil
}

func (c *JMXCheck) Interval() time.Duration {
	return 0
}

func (c *JMXCheck) ID() check.ID {
	return c.id
}

func (c *JMXCheck) GetWarnings() []error {
	return []error{}
}

func (c *JMXCheck) GetMetricStats() (map[string]int64, error) {
	return make(map[string]int64), nil
}
