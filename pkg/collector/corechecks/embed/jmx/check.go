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
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	telemetry_utils "github.com/DataDog/datadog-agent/pkg/telemetry/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// JMXCheck represents a JMXFetch check
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

// Run schedules this JMXCheck to run
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

// Stop forces the JMXCheck to stop and will unschedule it
func (c *JMXCheck) Stop() {
	close(c.stop)
	state.unscheduleCheck(c)
}

// Cancel is a noop
func (c *JMXCheck) Cancel() {}

// String provides a printable version of the JMXCheck
func (c *JMXCheck) String() string {
	return c.name
}

// Version returns the version of the JMXCheck
// (note, returns an empty string)
func (c *JMXCheck) Version() string {
	return ""
}

// ConfigSource returns the source of the configuration of the JMXCheck
func (c *JMXCheck) ConfigSource() string {
	return c.source
}

// InitConfig returns the init_config in YAML or JSON of the JMXCheck
func (c *JMXCheck) InitConfig() string {
	return c.initConfig
}

// InstanceConfig returns the metric config in YAML or JSON of the JMXCheck
func (c *JMXCheck) InstanceConfig() string {
	return c.instanceConfig
}

// Configure configures this JMXCheck, setting InitConfig and InstanceConfig
func (c *JMXCheck) Configure(integrationConfigDigest uint64, config integration.Data, initConfig integration.Data, source string) error {
	c.initConfig = string(config)
	c.instanceConfig = string(initConfig)
	return nil
}

// Interval returns the scheduling time for the check (0 for JMXCheck)
func (c *JMXCheck) Interval() time.Duration {
	return 0
}

// ID provides a unique identifier for this JMXCheck instance
func (c *JMXCheck) ID() check.ID {
	return c.id
}

// IsTelemetryEnabled returns if telemetry is enabled for this JMXCheck
func (c *JMXCheck) IsTelemetryEnabled() bool {
	return c.telemetry
}

// GetWarnings returns the last warning registered by this JMXCheck (currently an empty slice)
func (c *JMXCheck) GetWarnings() []error {
	return []error{}
}

// GetSenderStats returns the stats from the last run of this JMXCheck
func (c *JMXCheck) GetSenderStats() (check.SenderStats, error) {
	return check.NewSenderStats(), nil
}

// GetDiagnoses returns the diagnoses cached in last run or diagnose explicitly
func (c *JMXCheck) GetDiagnoses() ([]diagnosis.Diagnosis, error) {
	return nil, nil
}
