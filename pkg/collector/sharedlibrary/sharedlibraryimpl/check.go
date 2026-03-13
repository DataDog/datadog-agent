// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sharedlibrarycheck

package sharedlibrarycheck

import (
	"fmt"
	"time"

	yaml "go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check/defaults"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/collector/sharedlibrary/ffi"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Check is the definition of shared library checks
type Check struct {
	senderManager  sender.SenderManager
	id             checkid.ID
	version        string
	interval       time.Duration
	name           string
	libraryLoader  ffi.LibraryLoader // FFI handler
	lib            *ffi.Library      // handle of the associated shared library and pointers to its symbols
	source         string
	initConfig     string // json string of check init config
	instanceConfig string // json string of specific instance config
	cancelled      bool
}

func newCheck(senderManager sender.SenderManager, name string, libraryLoader ffi.LibraryLoader, lib *ffi.Library) (*Check, error) {
	check := &Check{
		senderManager: senderManager,
		interval:      defaults.DefaultCheckInterval,
		name:          name,
		libraryLoader: libraryLoader,
		lib:           lib,
	}

	return check, nil
}

// Run a shared library check
func (c *Check) Run() error {
	return c.runCheckImpl(true)
}

// runCheckImpl runs the check implementation with its Run symbol
// This function is created to allow passing the commitMetrics parameter (not possible due to the Check interface)
func (c *Check) runCheckImpl(commitMetrics bool) error {
	if c.cancelled {
		return fmt.Errorf("check %s is already cancelled", c.name)
	}

	// run the check through the library loader
	err := c.libraryLoader.Run(c.lib, string(c.id), c.initConfig, c.instanceConfig)
	if err != nil {
		return fmt.Errorf("Run failed: %w", err)
	}

	if commitMetrics {
		s, err := c.senderManager.GetSender(c.ID())
		if err != nil {
			return fmt.Errorf("Failed to retrieve a Sender instance: %w", err)
		}
		s.Commit()
	}

	return nil
}

// Stop does nothing
func (*Check) Stop() {}

// Cancel closes the associated shared library and prevents the check from running
func (c *Check) Cancel() {
	// don't close the lib again if the check is already cancelled
	if c.cancelled {
		return
	}

	err := c.libraryLoader.Close(c.lib)
	if err != nil {
		log.Errorf("Cancel failed: %s", err)
	}

	c.cancelled = true
}

// String representation (for debug and logging)
func (c *Check) String() string {
	return c.name
}

// Version returns the check version (either given by the shared library or "unversioned" otherwise)
func (c *Check) Version() string {
	return c.version
}

// IsTelemetryEnabled is not enabled
func (*Check) IsTelemetryEnabled() bool {
	return false
}

// ConfigSource returns the source of the configuration for this check
func (c *Check) ConfigSource() string {
	return c.source
}

// Loader returns the check loader
func (c *Check) Loader() string {
	return CheckLoaderName
}

// InitConfig returns the init_config configuration for the check
func (c *Check) InitConfig() string {
	return c.initConfig
}

// InstanceConfig returns the instance configuration for the check.
func (c *Check) InstanceConfig() string {
	return c.instanceConfig
}

// GetWarnings returns nothing
func (*Check) GetWarnings() []error {
	return []error{}
}

// Configure the shared library check from YAML data
func (c *Check) Configure(_ sender.SenderManager, integrationConfigDigest uint64, instanceConfig integration.Data, initConfig integration.Data, source string) error {
	c.id = checkid.BuildID(c.String(), integrationConfigDigest, instanceConfig, initConfig)

	commonOptions := integration.CommonInstanceConfig{}
	if err := yaml.Unmarshal(instanceConfig, &commonOptions); err != nil {
		log.Errorf("invalid instance section for check %s: %s", string(c.id), err)
		return err
	}

	// See if a collection interval was specified
	if commonOptions.MinCollectionInterval > 0 {
		c.interval = time.Duration(commonOptions.MinCollectionInterval) * time.Second
	}

	// configuration fields
	c.source = source
	c.initConfig = string(initConfig)
	c.instanceConfig = string(instanceConfig)

	return nil
}

// GetSenderStats returns the stats from the last run of the check
func (c *Check) GetSenderStats() (stats.SenderStats, error) {
	sender, err := c.senderManager.GetSender(c.ID())
	if err != nil {
		return stats.SenderStats{}, fmt.Errorf("Failed to retrieve a Sender instance: %w", err)
	}
	return sender.GetSenderStats(), nil
}

// Interval returns the interval between each check execution
func (c *Check) Interval() time.Duration {
	return c.interval
}

// ID returns the ID of the check
func (c *Check) ID() checkid.ID {
	return checkid.ID(c.id)
}

// GetDiagnoses returns nothing
func (*Check) GetDiagnoses() ([]diagnose.Diagnosis, error) {
	return nil, nil
}

// IsHASupported does not apply to shared library checks
func (*Check) IsHASupported() bool {
	return false
}
