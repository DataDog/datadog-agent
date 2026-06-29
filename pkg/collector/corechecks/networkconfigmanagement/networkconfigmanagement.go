// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ncm

// Package networkconfigmanagement defines the agent core check for retrieving network device configurations
package networkconfigmanagement

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	networkconfigmanagement "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	ncmconfig "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/config"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// CheckName is the name of the check
const CheckName = "network_config_management"

// Check is the main struct for the network configuration management check
type Check struct {
	core.CheckBase
	checkContext *ncmconfig.NcmCheckContext
	sender       sender.Sender
	agentConfig  config.Component
	ncmComp      networkconfigmanagement.Component
}

// Run executes the check to retrieve network device configurations from a device
func (c *Check) Run() error {
	return c.ncmComp.ReportConfig(context.Background(), c.checkContext.Device.DeviceID(), c.sender)
}

// Configure sets up the check with the provided configuration and sender manager
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string, provider string) error {
	var err error

	// Load/parse the configuration for the device instance
	c.checkContext, err = ncmconfig.NewNcmCheckContext(rawInstance, rawInitConfig)
	if err != nil {
		return fmt.Errorf("build config failed: %w", err)
	}

	// Must be called before v.CommonConfigure
	c.BuildID(integrationConfigDigest, rawInstance, rawInitConfig)
	err = c.CommonConfigure(senderManager, rawInitConfig, rawInstance, source, provider)
	if err != nil {
		return fmt.Errorf("common configure failed: %w", err)
	}

	s, err := c.GetSender()
	if err != nil {
		return err
	}

	c.sender = s

	if err := c.ncmComp.RegisterDevice(c.checkContext.Device); err != nil {
		return fmt.Errorf("unable to register device %s: %w", c.checkContext.Device.DeviceID(), err)
	}
	c.ncmComp.SetMaxReportInterval(c.checkContext.InventoryReportMaxInterval)

	return nil
}

// Interval returns the interval at which the check should run (default 15 minutes for now)
func (c *Check) Interval() time.Duration {
	return c.checkContext.MinCollectionInterval
}

// Factory creates a new check factory
func Factory(agentConfig config.Component, ncmComp option.Option[networkconfigmanagement.Component]) option.Option[func() check.Check] {
	if comp, ok := ncmComp.Get(); ok {
		return option.New(func() check.Check {
			return newCheck(agentConfig, comp)
		})
	}
	return option.None[func() check.Check]()
}

// newCheck creates a new instance of the Check with the provided agent configuration
func newCheck(agentConfig config.Component, ncmComp networkconfigmanagement.Component) check.Check {
	return &Check{
		CheckBase:   core.NewCheckBase(CheckName),
		agentConfig: agentConfig,
		ncmComp:     ncmComp,
	}
}
