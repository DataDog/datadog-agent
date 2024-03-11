// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package ciscosdwan implements NDM Cisco SD-WAN corecheck
package ciscosdwan

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/client"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/report"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
	"gopkg.in/yaml.v2"
)

const (
	// CheckName is the name of the check
	CheckName = "ciscosdwan"
)

// Configuration for the Cisco SD-WAN check
type checkCfg struct {
	Hostname string `yaml:"hostname"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	UseHTTPS bool   `yaml:"use_https"`
	Insecure bool   `yaml:"insecure"`
}

// CiscoSdwanCheck contains the field for the CiscoSdwanCheck
type CiscoSdwanCheck struct {
	core.CheckBase
	config        checkCfg
	client        *client.Client
	metricsSender *report.SDWanSender
}

// Run executes the check
func (c *CiscoSdwanCheck) Run() error {
	log.Info("Running Cisco SD-WAN check")

	devices, err := c.client.GetDevices()
	if err != nil {
		return err
	}
	vEdgeInterfaces, err := c.client.GetVEdgeInterfaces()
	if err != nil {
		return err
	}
	cEdgeInterfaces, err := c.client.GetCEdgeInterfaces()
	if err != nil {
		return err
	}
	deviceStats, err := c.client.GetDeviceHardwareMetrics()
	if err != nil {
		return err
	}
	interfaceStats, err := c.client.GetInterfacesMetrics()
	if err != nil {
		return err
	}
	appRouteStats, err := c.client.GetApplicationAwareRoutingMetrics()
	if err != nil {
		return err
	}
	controlConnectionsState, err := c.client.GetControlConnectionsState()
	if err != nil {
		return err
	}
	ompPeersState, err := c.client.GetOMPPeersState()
	if err != nil {
		return err
	}
	bfdSessionsState, err := c.client.GetBFDSessionsState()
	if err != nil {
		return err
	}
	deviceCounters, err := c.client.GetDevicesCounters()
	if err != nil {
		return err
	}

	uptimes := process.ProcessDeviceUptimes(devices)

	c.metricsSender.SendMetadata(devices, vEdgeInterfaces, cEdgeInterfaces)

	c.metricsSender.SendDeviceMetrics(deviceStats)
	c.metricsSender.SendInterfaceMetrics(interfaceStats)
	c.metricsSender.SendUptimeMetrics(uptimes)
	c.metricsSender.SendAppRouteMetrics(appRouteStats)
	c.metricsSender.SendControlConnectionMetrics(controlConnectionsState)
	c.metricsSender.SendOMPPeerMetrics(ompPeersState)
	c.metricsSender.SendBFDSessionMetrics(bfdSessionsState)
	c.metricsSender.SendDeviceCountersMetrics(deviceCounters)

	// Commit
	c.metricsSender.Commit()
	log.Info("Done running Cisco SD-WAN check")

	return nil
}

// Configure the Viptela check
func (c *CiscoSdwanCheck) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string) error {
	err := c.CommonConfigure(senderManager, integrationConfigDigest, rawInitConfig, rawInstance, source)
	if err != nil {
		return err
	}

	var instanceConfig checkCfg
	err = yaml.Unmarshal(rawInstance, &instanceConfig)
	if err != nil {
		return err
	}

	// TEMPORARY Create dogstatsd Client
	statsdHost := config.GetBindHost()
	statsdPort := config.Datadog.GetInt("dogstatsd_port")
	statsdAddr := fmt.Sprintf("%s:%d", statsdHost, statsdPort)
	log.Infof("Statsd Addr: %s", statsdAddr)

	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	statsdClient, err := ddgostatsd.New(statsdAddr)
	if err != nil {
		log.Errorf("Error creating statsd Client: %s", err)
	}

	// Creating Cisco SD-WAN API client
	c.client, err = client.NewClient(instanceConfig.Hostname, instanceConfig.Username, instanceConfig.Password, instanceConfig.UseHTTPS, instanceConfig.Insecure)
	if err != nil {
		return err
	}

	c.metricsSender = report.NewSDWanSender(sender, statsdClient)

	return nil
}

// Factory creates a new check factory
func Factory() optional.Option[func() check.Check] {
	return optional.NewOption(newCheck)
}

func newCheck() check.Check {
	return &CiscoSdwanCheck{
		CheckBase: core.NewCheckBase(CheckName),
	}
}
