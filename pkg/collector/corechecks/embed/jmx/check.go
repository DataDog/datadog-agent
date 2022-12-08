// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jmx
// +build jmx

package jmx

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	telemetry_utils "github.com/DataDog/datadog-agent/pkg/telemetry/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// JMXCheck TODO <agent-core> : IML-199
type JMXCheck struct {
	id             check.ID
	name           string
	config         integration.Config
	stop           chan struct{}
	source         string
	telemetry      bool
	initConfig     string
	instanceConfig string
}

func newJMXCheck(config integration.Config, source string) *JMXCheck {
	digest := config.IntDigest()
	check := &JMXCheck{
		config:    config,
		stop:      make(chan struct{}),
		name:      config.Name,
		id:        check.ID(fmt.Sprintf("%v_%x", config.Name, digest)),
		source:    source,
		telemetry: telemetry_utils.IsCheckEnabled("jmx"),
	}
	check.Configure(digest, config.InitConfig, config.MetricConfig, source) //nolint:errcheck

	return check
}

// Run TODO <agent-core> : IML-199
func (c *JMXCheck) Run() error {
	err := state.scheduleCheck(c)
	if err != nil {
		return err
	}

	select {
	case <-state.runnerError:
		return fmt.Errorf("jmxfetch exited, stopping %s : %s", c.name, err)
	case <-c.stop:
		log.Infof("jmx check %s stopped", c.name)
	}

	return nil
}

// Stop TODO <agent-core> : IML-199
func (c *JMXCheck) Stop() {
	close(c.stop)
	state.unscheduleCheck(c)
}

// Cancel TODO <agent-core> : IML-199
func (c *JMXCheck) Cancel() {}

// String TODO <agent-core> : IML-199
func (c *JMXCheck) String() string {
	return c.name
}

// Version TODO <agent-core> : IML-199
func (c *JMXCheck) Version() string {
	return ""
}

// ConfigSource TODO <agent-core> : IML-199
func (c *JMXCheck) ConfigSource() string {
	return c.source
}

// InitConfig TODO <agent-core> : IML-199
func (c *JMXCheck) InitConfig() string {
	return c.initConfig
}

// InstanceConfig TODO <agent-core> : IML-199
func (c *JMXCheck) InstanceConfig() string {
	return c.instanceConfig
}

// Configure TODO <agent-core> : IML-199
func (c *JMXCheck) Configure(integrationConfigDigest uint64, config integration.Data, initConfig integration.Data, source string) error {
	c.initConfig = string(config)
	c.instanceConfig = string(initConfig)
	return nil
}

// Interval TODO <agent-core> : IML-199
func (c *JMXCheck) Interval() time.Duration {
	return 0
}

// ID TODO <agent-core> : IML-199
func (c *JMXCheck) ID() check.ID {
	return c.id
}

// IsTelemetryEnabled TODO <agent-core> : IML-199
func (c *JMXCheck) IsTelemetryEnabled() bool {
	return c.telemetry
}

// GetWarnings TODO <agent-core> : IML-199
func (c *JMXCheck) GetWarnings() []error {
	return []error{}
}

// GetSenderStats TODO <agent-core> : IML-199
func (c *JMXCheck) GetSenderStats() (check.SenderStats, error) {
	return check.NewSenderStats(), nil
}
