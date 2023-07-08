// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package netpath

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/netpath/traceroute"
	"github.com/shirou/gopsutil/v3/cpu"
	"net"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

const checkName = "netpath"

// Check doesn't need additional fields
type Check struct {
	core.CheckBase
	nbCPU       float64
	lastNbCycle float64
	lastTimes   cpu.TimesStat
	config      *CheckConfig
}

// Run executes the check
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	c.traceroute(sender)

	sender.Gauge("netpath.test_metric", 10, "", nil)
	sender.Commit()
	return nil
}

func (c *Check) traceroute(sender sender.Sender) {
	options := traceroute.TracerouteOptions{}
	options.SetRetries(1)
	options.SetMaxHops(30)
	//options.SetFirstHop(traceroute.DEFAULT_FIRST_HOP)
	times := 1

	hosts := []string{c.config.Hostname}

	var allHops [][]traceroute.TracerouteHop
	for _, host := range hosts {

		ipAddr, err := net.ResolveIPAddr("ip", host)
		if err != nil {
			return
		}

		fmt.Printf("traceroute to %v (%v), %v hops max, %v byte packets\n", host, ipAddr, options.MaxHops(), options.PacketSize())

		hostHops := getHops(options, times, err, host)
		printHops(hostHops)
		allHops = append(allHops, hostHops...)
	}

}

// Configure the CPU check
func (c *Check) Configure(integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	err := c.CommonConfigure(integrationConfigDigest, initConfig, data, source)
	if err != nil {
		return err
	}
	config, err := NewCheckConfig(data, initConfig)
	if err != nil {
		return err
	}
	c.config = config
	return nil
}

func netpathFactory() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(checkName),
	}
}

func init() {
	core.RegisterCheck(checkName, netpathFactory)
}
