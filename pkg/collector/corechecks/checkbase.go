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
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
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
	initConfig     string
	instanceConfig string
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
func (c *CheckBase) BuildID(integrationConfigDigest uint64, instance, initConfig integration.Data) {
	c.checkID = check.BuildID(c.checkName, integrationConfigDigest, instance, initConfig)
}

// Configure is provided for checks that require no config. If overridden,
// the call to CommonConfigure must be preserved.
func (c *CheckBase) Configure(integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	err := c.CommonConfigure(integrationConfigDigest, initConfig, data, source)
	if err != nil {
		return err
	}

	// Add the possibly configured service as a tag for this check
	s, err := c.GetSender()
	if err != nil {
		log.Errorf("failed to retrieve a sender for check %s: %s", string(c.ID()), err)
		return err
	}
	s.FinalizeCheckServiceTag()

	return nil
}

// CommonConfigure is called when checks implement their own Configure method,
// in order to setup common options (run interval, empty hostname)
func (c *CheckBase) CommonConfigure(integrationConfigDigest uint64, initConfig, instanceConfig integration.Data, source string) error {
	handleConf := func(conf integration.Data, c *CheckBase) error {
		commonOptions := integration.CommonInstanceConfig{}
		err := yaml.Unmarshal(conf, &commonOptions)
		if err != nil {
			log.Errorf("invalid configuration section for check %s: %s", string(c.ID()), err)
			return err
		}

		// See if a collection interval was specified
		if commonOptions.MinCollectionInterval > 0 {
			c.checkInterval = time.Duration(commonOptions.MinCollectionInterval) * time.Second
		}

		// Disable default hostname if specified
		if commonOptions.EmptyDefaultHostname {
			s, err := c.GetSender()
			if err != nil {
				log.Errorf("failed to retrieve a sender for check %s: %s", string(c.ID()), err)
				return err
			}
			s.DisableDefaultHostname(true)
		}

		// Set custom tags configured for this check
		if len(commonOptions.Tags) > 0 {
			s, err := c.GetSender()
			if err != nil {
				log.Errorf("failed to retrieve a sender for check %s: %s", string(c.ID()), err)
				return err
			}
			s.SetCheckCustomTags(commonOptions.Tags)
		}

		// Set configured service for this check, overriding the one possibly defined globally
		if len(commonOptions.Service) > 0 {
			s, err := c.GetSender()
			if err != nil {
				log.Errorf("failed to retrieve a sender for check %s: %s", string(c.ID()), err)
				return err
			}
			s.SetCheckService(commonOptions.Service)
		}

		c.source = source
		return nil
	}
	if err := handleConf(initConfig, c); err != nil {
		return err
	}
	if err := handleConf(instanceConfig, c); err != nil {
		return err
	}

	c.initConfig = string(initConfig)
	c.instanceConfig = string(instanceConfig)
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

// Cancel does nothing by default. Override it if your check has
// background resources that need to be cleaned up when the check is
// unscheduled.
func (c *CheckBase) Cancel() {
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

// InitConfig returns the init_config configuration for the check.
func (c *CheckBase) InitConfig() string {
	return c.initConfig
}

// InstanceConfig returns the instance configuration for the check.
func (c *CheckBase) InstanceConfig() string {
	return c.instanceConfig
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

// GetSender gets the object to which metrics for this check should be sent.
//
// This is a "safe" sender, specialized to avoid some common errors, at a very
// small cost to performance.  Performance-sensitive checks can use GetRawSender()
// to avoid this performance cost, as long as they are careful to avoid errors.
//
// See `safesender.go` for details on the managed errors.
func (c *CheckBase) GetSender() (aggregator.Sender, error) {
	sender, err := c.GetRawSender()
	if err != nil {
		return nil, err
	}
	return newSafeSender(sender), err
}

// GetRawSender is similar to GetSender, but does not provide the safety wrapper.
func (c *CheckBase) GetRawSender() (aggregator.Sender, error) {
	return aggregator.GetSender(c.ID())
}

// GetSenderStats returns the stats from the last run of the check.
func (c *CheckBase) GetSenderStats() (check.SenderStats, error) {
	sender, err := c.GetSender()
	if err != nil {
		return check.SenderStats{}, fmt.Errorf("failed to retrieve a sender: %v", err)
	}
	return sender.GetSenderStats(), nil
}

// GetDiagnoses returns the diagnoses cached in last run or diagnose explicitly
func (c *CheckBase) GetDiagnoses() ([]diagnosis.Diagnosis, error) {
	return nil, nil
}
