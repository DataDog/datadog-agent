// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package networkconfigmanagement defines the agent core check for retrieving network device configurations
package networkconfigmanagement

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/snmp/utils"

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

	"gopkg.in/yaml.v2"
)

// CheckName is the name of the check
const CheckName = "network_config_management"

// Check is the main struct for the network configuration management check
type Check struct {
	core.CheckBase
	deviceConfig *ncmconfig.DeviceConfig
	sender       *ncmsender.NCMSender
	agentConfig  config.Component
	remoteClient ncmremote.Client
}

// TimeNow useful for mocking
var TimeNow = time.Now

// Run executes the check to retrieve network device configurations from a device
func (c *Check) Run() error {
	var checkErr error
	var configs []ncmreport.NetworkDeviceConfig

	checkErr = c.remoteClient.Connect()
	if checkErr != nil {
		log.Errorf("unable to connect to remote device %s: %s", c.deviceConfig.IPAddress, checkErr)
		return checkErr
	}
	defer func() {
		if c.remoteClient != nil {
			_ = c.remoteClient.Close()
		}
	}()

	// TODO: validate the running config to make sure it's valid, extract other information from it, etc.
	runningConfig, checkErr := c.remoteClient.RetrieveRunningConfig()
	if checkErr != nil {
		return checkErr
	}

	deviceID := fmt.Sprintf("%s:%s", c.deviceConfig.Namespace, c.deviceConfig.IPAddress)
	tags := []string{
		"device_ip:" + c.deviceConfig.IPAddress,
	}
	configs = append(configs, ncmreport.ToNetworkDeviceConfig(deviceID, c.deviceConfig.IPAddress, ncmreport.RUNNING, TimeNow().Unix(), tags, runningConfig))

	if c.deviceConfig.CollectStartupConfig {
		// TODO: validate the startup config to make sure it's valid, extract other information from it, etc.
		startupConfig, checkErr := c.remoteClient.RetrieveStartupConfig()
		if checkErr != nil {
			return checkErr
		}
		configs = append(configs, ncmreport.ToNetworkDeviceConfig(deviceID, c.deviceConfig.IPAddress, ncmreport.STARTUP, TimeNow().Unix(), tags, startupConfig))
	}

	checkErr = c.sender.SendNCMConfig(ncmreport.ToNCMPayload(c.deviceConfig.Namespace, "", configs, TimeNow().Unix()))
	if checkErr != nil {
		return checkErr
	}

	// TODO: Send any metrics as well
	//c.sender.SendNCMMetrics()

	c.sender.Commit()
	return nil
}

// Configure sets up the check with the provided configuration and sender manager
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string) error {
	var err error
	// Must be called before v.CommonConfigure
	c.BuildID(integrationConfigDigest, rawInstance, rawInitConfig)
	err = c.CommonConfigure(senderManager, rawInitConfig, rawInstance, source)
	if err != nil {
		return fmt.Errorf("common configure failed: %s", err)
	}

	var deviceConfig ncmconfig.DeviceConfig
	err = yaml.Unmarshal(rawInstance, &deviceConfig)
	if err != nil {
		return err
	}
	err = deviceConfig.ValidateConfig()
	if err != nil {
		return err
	}
	c.deviceConfig = &deviceConfig

	if c.deviceConfig.Namespace == "" {
		c.deviceConfig.Namespace = "default"
	} else {
		namespace, err := utils.NormalizeNamespace(c.deviceConfig.Namespace)
		if err != nil {
			return err
		}
		c.deviceConfig.Namespace = namespace
	}

	// TODO: set up init config (namespace?)

	s, err := c.GetSender()
	if err != nil {
		return err
	}
	ncmSender := ncmsender.NewNCMSender(s, c.deviceConfig.Namespace)
	c.sender = ncmSender

	// TODO: add check to see the device's credentials type (SSH/Telnet) and create appropriate client factory
	c.remoteClient = ncmremote.NewSSHClient(c.deviceConfig)

	return nil
}

// Interval returns the interval at which the check should run (default 15 minutes for now)
func (c *Check) Interval() time.Duration {
	return time.Minute * 15
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
