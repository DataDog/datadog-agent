// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

package corechecks

import (
	"fmt"
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/check/defaults"
	telemetry_utils "github.com/DataDog/datadog-agent/pkg/telemetry/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// CheckBase provides default implementations for most of the check.Check
// interface to make it easier to bootstrap a new corecheck.
//
// To use it, you need to embed it in your check struct, by calling
// NewCheckBase() in your factory, plus:
// - long-running checks must override Stop() and Interval()
// - checks supporting multiple instances must call BuildID() from
// their Configure() method
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
		telemetry:     telemetry_utils.IsCheckEnabled(name),
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
	commonGlobalOptions := integration.CommonGlobalConfig{}
	err := yaml.Unmarshal(initConfig, &commonGlobalOptions)
	if err != nil {
		log.Errorf("invalid init_config section for check %s: %s", string(c.ID()), err)
		return err
	}

	// Set service for this check
	if len(commonGlobalOptions.Service) > 0 {
		s, err := aggregator.GetSender(c.checkID)
		if err != nil {
			log.Errorf("failed to retrieve a sender for check %s: %s", string(c.ID()), err)
			return err
		}
		s.SetCheckService(commonGlobalOptions.Service)
	}

	err = c.CommonConfigure(data, source)
	if err != nil {
		return err
	}

	// Add the possibly configured service as a tag for this check
	s, err := aggregator.GetSender(c.checkID)
	if err != nil {
		log.Errorf("failed to retrieve a sender for check %s: %s", string(c.ID()), err)
		return err
	}
	s.FinalizeCheckServiceTag()

	return nil
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

	// Set configured service for this check, overriding the one possibly defined globally
	if len(commonOptions.Service) > 0 {
		s, err := aggregator.GetSender(c.checkID)
		if err != nil {
			log.Errorf("failed to retrieve a sender for check %s: %s", string(c.ID()), err)
			return err
		}
		s.SetCheckService(commonOptions.Service)
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

// Cancel calls CommonCancel by default. Override it if
// your check has background resources that need to be cleaned up
// when the check is unscheduled. Make sure to call CommonCancel from
// your override.
func (c *CheckBase) Cancel() {
	c.CommonCancel()
}

// CommonCancel cleans up common resources. Must be called from Cancel
// when checks implement it.
func (c *CheckBase) CommonCancel() {
	aggregator.DestroySender(c.checkID)
}

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

// GetSenderStats returns the stats from the last run of the check.
func (c *CheckBase) GetSenderStats() (check.SenderStats, error) {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return check.SenderStats{}, fmt.Errorf("failed to retrieve a sender: %v", err)
	}
	return sender.GetSenderStats(), nil
}
