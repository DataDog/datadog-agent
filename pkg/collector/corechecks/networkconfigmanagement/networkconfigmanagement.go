// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ncm

// Package networkconfigmanagement defines the agent core check for retrieving network device configurations
package networkconfigmanagement

import (
	"fmt"
	"slices"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	"github.com/benbjohnson/clock"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	ncmconfig "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/config"
	ncmremote "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/remote"
	ncmreport "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/report"
	ncmsender "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/sender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// CheckName is the name of the check
const CheckName = "network_config_management"

// Check is the main struct for the network configuration management check
type Check struct {
	core.CheckBase
	checkContext  *ncmconfig.NcmCheckContext
	sender        *ncmsender.NCMSender
	agentConfig   config.Component
	remoteClient  ncmremote.Client
	clock         clock.Clock
	lastCheckTime time.Time
}

// Run executes the check to retrieve network device configurations from a device
func (c *Check) Run() error {
	var checkErr error
	var configs []ncmreport.NetworkDeviceConfig
	checkStartTime := c.clock.Now()

	checkErr = c.remoteClient.Connect()
	if checkErr != nil {
		log.Errorf("unable to connect to remote device %s: %s", c.checkContext.Device.IPAddress, checkErr)
		return checkErr
	}

	// Must defer this way because sometimes we will have to redial the remote client
	defer func() {
		if c.remoteClient != nil {
			_ = c.remoteClient.Close()
		}
	}()

	// If the check did not have inline profile explicitly defined/from cache, find the profile that works
	if !c.checkContext.ProfileCache.HasSetProfile() {
		prof, err := c.FindMatchingProfile()
		if err != nil {
			return err
		}
		c.checkContext.ProfileCache.Profile = prof
		c.checkContext.ProfileCache.ProfileName = prof.Name
	}
	// Update the remote client's device profile to access the correct commands
	c.remoteClient.SetProfile(c.checkContext.ProfileCache.Profile)

	deviceID := fmt.Sprintf("%s:%s", c.checkContext.Namespace, c.checkContext.Device.IPAddress)
	deviceTags := c.getDeviceTags()
	c.sender.SetDeviceTags(deviceTags)

	rawRunningConfig, checkErr := c.remoteClient.RetrieveRunningConfig()
	if checkErr != nil {
		return checkErr
	}

	runningConfig, metadata, checkErr := c.checkContext.ProfileCache.Profile.ProcessCommandOutput(profile.Running, rawRunningConfig)
	if checkErr != nil {
		log.Warnf("unable to process rules for running config for device %s, using agent collection ts: %s", deviceID, checkErr)
	}
	// TODO: helper fn to take metadata that needs to be emitted as metrics + emit them
	configs = append(configs, ncmreport.ToNetworkDeviceConfig(deviceID, c.checkContext.Device.IPAddress, ncmreport.RUNNING, metadata, deviceTags, runningConfig))

	rawStartupConfig, checkErr := c.remoteClient.RetrieveStartupConfig()
	if checkErr != nil {
		// If the startup config cannot be retrieved, log a warning but continue
		log.Warnf("unable to retrieve startup config for %s, will not send: %s", deviceID, checkErr)
	} else {
		startupConfig, metadata, checkErr := c.checkContext.ProfileCache.Profile.ProcessCommandOutput(profile.Startup, rawStartupConfig)
		if checkErr != nil {
			log.Warnf("unable to process rules for startup config for device %s, using agent collection ts: %s", deviceID, checkErr)
		}
		// add the startup config to the payload if it was retrieved successfully
		configs = append(configs, ncmreport.ToNetworkDeviceConfig(deviceID, c.checkContext.Device.IPAddress, ncmreport.STARTUP, metadata, deviceTags, startupConfig))
	}

	checkErr = c.sender.SendNCMConfig(ncmreport.ToNCMPayload(c.checkContext.Namespace, configs, c.clock.Now().Unix()))
	if checkErr != nil {
		return checkErr
	}

	c.sender.SendNCMCheckMetrics(checkStartTime, c.lastCheckTime)
	c.lastCheckTime = checkStartTime

	c.sender.Commit()
	return nil
}

// Configure sets up the check with the provided configuration and sender manager
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string) error {
	var err error

	// Load/parse the configuration for the device instance
	c.checkContext, err = ncmconfig.NewNcmCheckContext(rawInstance, rawInitConfig)
	if err != nil {
		return fmt.Errorf("build config failed: %w", err)
	}

	// Must be called before v.CommonConfigure
	c.BuildID(integrationConfigDigest, rawInstance, rawInitConfig)
	err = c.CommonConfigure(senderManager, rawInitConfig, rawInstance, source)
	if err != nil {
		return fmt.Errorf("common configure failed: %w", err)
	}

	// Initialize the clock
	c.clock = clock.New()

	// Initialize the Sender
	s, err := c.GetSender()
	if err != nil {
		return err
	}
	ncmSender := ncmsender.NewNCMSender(s, c.checkContext.Namespace, c.clock)
	c.sender = ncmSender

	// TODO: add check to see the device's credentials type (SSH/Telnet) and create appropriate client factory
	c.remoteClient, err = ncmremote.NewSSHClient(c.checkContext.Device)
	if err != nil {
		return fmt.Errorf("create remote SSH client failed: %w", err)
	}

	return nil
}

// Interval returns the interval at which the check should run (default 15 minutes for now)
func (c *Check) Interval() time.Duration {
	return c.checkContext.MinCollectionInterval
}

// Factory creates a new check factory
func Factory(agentConfig config.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(agentConfig)
	})
}

// newCheck creates a new instance of the Check with the provided agent configuration
func newCheck(agentConfig config.Component) check.Check {
	return &Check{
		CheckBase:   core.NewCheckBase(CheckName),
		agentConfig: agentConfig,
	}
}

// FindMatchingProfile supports testing profiles until one is found with a successful command for the device
func (c *Check) FindMatchingProfile() (*profile.NCMProfile, error) {
	for profName, prof := range c.checkContext.ProfileMap {
		if c.checkContext.ProfileCache.HasTried(profName) {
			continue
		}
		c.remoteClient.SetProfile(prof)
		_, err := c.remoteClient.RetrieveRunningConfig()
		if err != nil {
			log.Warnf("error with running config retrieval on profile %s on remote device %s: %s", profName, c.checkContext.Device.IPAddress, err)
			c.checkContext.ProfileCache.AppendToTriedProfiles(profName)
			// TODO: clear the profile if it didn't work
			continue
		}
		return prof, nil
	}
	return nil, fmt.Errorf("unable to find matching profile for device %s", c.checkContext.Device.IPAddress)
}

func (c *Check) getDeviceTags() []string {
	deviceID := fmt.Sprintf("%s:%s", c.checkContext.Namespace, c.checkContext.Device.IPAddress)
	deviceTags := []string{
		"device_namespace:" + c.checkContext.Namespace,
		"device_ip:" + c.checkContext.Device.IPAddress,
		"device_id:" + deviceID,
		// TODO: device_hostname - may need to be extracted from config / output to be retrieved in NCM core check
		"config_source:cli",
		"profile:" + c.checkContext.ProfileCache.ProfileName,
	}
	return slices.Clone(deviceTags)
}
