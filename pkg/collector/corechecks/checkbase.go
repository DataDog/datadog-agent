// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package corechecks

import (
	"fmt"
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/check/defaults"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// CheckBase provides default implementations for most of the check.Check
// interface to make it easier to bootstrap a new corecheck.
//
// To use it, you need to embed it in your check struct, by calling
// NewCheckBase() in your factory, plus:
// - long-running checks must override Stop() and Interval()
// - checks supporting multiple instances must call BuildID() from
// their Config() method
// - after optionally building a unique ID, CommonConfigure() must
// be called from the Config() method to handle the common instance
// fields
//
// Integration warnings are handled via the Warn and Warnf methods
// that forward the warning to the logger and send the warning to
// the collector for display in the status page and the web UI.
//
// If custom tags are set in the instance configuration, they will
// be automatically appended to each send done by this check.
type CheckBase struct {
	checkName      string
	checkID        check.ID
	latestWarnings []error
	checkInterval  time.Duration
	source         string
	telemetry      bool
}

// NewCheckBase returns a check base struct with a given check name
func NewCheckBase(name string) CheckBase {
	return NewCheckBaseWithInterval(name, defaults.DefaultCheckInterval)
}

// NewCheckBaseWithInterval returns a check base struct with a given check name and interval
func NewCheckBaseWithInterval(name string, defaultInterval time.Duration) CheckBase {
	return CheckBase{
		checkName:     name,
		checkID:       check.ID(name),
		checkInterval: defaultInterval,
		telemetry:     telemetry.IsCheckEnabled(name),
	}
}

// BuildID is to be called by the check's Config() method to generate
// the unique check ID.
func (c *CheckBase) BuildID(instance, initConfig integration.Data) {
	c.checkID = check.BuildID(c.checkName, instance, initConfig)
}

// Configure is provided for checks that require no config. If overridden,
// the call to CommonConfigure must be preserved.
func (c *CheckBase) Configure(data integration.Data, initConfig integration.Data, source string) error {
	return c.CommonConfigure(data, source)
}

// CommonConfigure is called when checks implement their own Configure method,
// in order to setup common options (run interval, empty hostname)
func (c *CheckBase) CommonConfigure(instance integration.Data, source string) error {
	commonOptions := integration.CommonInstanceConfig{}
	err := yaml.Unmarshal(instance, &commonOptions)
	if err != nil {
		log.Errorf("invalid instance section for check %s: %s", string(c.ID()), err)
		return err
	}

	// See if a collection interval was specified
	if commonOptions.MinCollectionInterval > 0 {
		c.checkInterval = time.Duration(commonOptions.MinCollectionInterval) * time.Second
	}

	// Disable default hostname if specified
	if commonOptions.EmptyDefaultHostname {
		s, err := aggregator.GetSender(c.checkID)
		if err != nil {
			log.Errorf("failed to retrieve a sender for check %s: %s", string(c.ID()), err)
			return err
		}
		s.DisableDefaultHostname(true)
	}

	// Set custom tags configured for this check
	if len(commonOptions.Tags) > 0 {
		s, err := aggregator.GetSender(c.checkID)
		if err != nil {
			log.Errorf("failed to retrieve a sender for check %s: %s", string(c.ID()), err)
			return err
		}
		s.SetCheckCustomTags(commonOptions.Tags)
	}

	c.source = source
	return nil
}

// Warn sends an integration warning to logs + agent status.
func (c *CheckBase) Warn(v ...interface{}) error {
	w := log.Warn(v...)
	c.latestWarnings = append(c.latestWarnings, w)

	return w
}

// Warnf sends an integration warning to logs + agent status.
func (c *CheckBase) Warnf(format string, params ...interface{}) error {
	w := log.Warnf(format, params...)
	c.latestWarnings = append(c.latestWarnings, w)

	return w
}

// Stop does nothing by default, you need to implement it in
// long-running checks (persisting after Run() exits)
func (c *CheckBase) Stop() {}

// Interval returns the scheduling time for the check.
// Long-running checks should override to return 0.
func (c *CheckBase) Interval() time.Duration {
	return c.checkInterval
}

// String returns the name of the check, the same for every instance
func (c *CheckBase) String() string {
	return c.checkName
}

// Version returns an empty string as Go check can't be updated independently
// from the agent
func (c *CheckBase) Version() string {
	return ""
}

// ConfigSource returns an empty string as Go check can't be updated independently
// from the agent
func (c *CheckBase) ConfigSource() string {
	return c.source
}

// ID returns a unique ID for that check instance
//
// For checks that only support one instance, the default value is
// the check name. Regular checks must call BuildID() from Config()
// to build their ID.
func (c *CheckBase) ID() check.ID {
	return c.checkID
}

// IsTelemetryEnabled returns if the telemetry is enabled for this check.
func (c *CheckBase) IsTelemetryEnabled() bool {
	return c.telemetry
}

// GetWarnings grabs the latest integration warnings for the check.
func (c *CheckBase) GetWarnings() []error {
	if len(c.latestWarnings) == 0 {
		return nil
	}
	w := c.latestWarnings
	c.latestWarnings = []error{}
	return w
}

// GetMetricStats returns the stats from the last run of the check.
func (c *CheckBase) GetMetricStats() (map[string]int64, error) {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve a sender: %v", err)
	}
	return sender.GetMetricStats(), nil
}
