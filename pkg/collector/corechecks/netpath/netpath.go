// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package netpath

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/netpath/traceroute"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v3/cpu"
	"net"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

const checkName = "netpath"

// TODO: FIXME The mutex is used to prevent multiple checks running at the same
//
//	It seems there are some concurrency issues
var globalMu = &sync.Mutex{}

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
	globalMu.Lock()
	defer globalMu.Unlock()
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	err = c.traceroute(sender)
	if err != nil {
		return err
	}

	sender.Gauge("netpath.test_metric", 10, "", nil)
	sender.Commit()
	return nil
}

func (c *Check) traceroute(sender sender.Sender) error {
	options := traceroute.TracerouteOptions{}
	options.SetRetries(1)
	options.SetMaxHops(15)
	//options.SetFirstHop(traceroute.DEFAULT_FIRST_HOP)
	times := 1
	destinationHost := c.config.Hostname

	hname, err := hostname.Get(context.TODO())
	if err != nil {
		return err
	}

	ipAddr, err := net.ResolveIPAddr("ip", destinationHost)
	if err != nil {
		return nil
	}

	fmt.Printf("traceroute to %v (%v), %v hops max, %v byte packets\n", destinationHost, ipAddr, options.MaxHops(), options.PacketSize())

	hostHops := getHops(options, times, err, destinationHost)
	if len(hostHops) == 0 {
		return errors.New("no hops")
	}

	err = c.traceRouteV1(sender, hostHops, hname, destinationHost)
	if err != nil {
		return err
	}
	for i := 0; i < 100; i++ {
		err = c.traceRouteV2(sender, hostHops, hname, destinationHost)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Check) traceRouteV1(sender sender.Sender, hostHops [][]traceroute.TracerouteHop, hname string, destinationHost string) error {
	tr := NewTraceroute()
	tr.Timestamp = time.Now().UnixMilli()
	tr.AgentHost = hname
	tr.DestinationHost = destinationHost

	hops := hostHops[0]
	for _, hop := range hops {
		ip := hop.AddressString()
		hop := TracerouteHop{
			TTL:       hop.TTL,
			IpAddress: ip,
			Host:      hop.HostOrAddressString(),
			Duration:  hop.ElapsedTime.Seconds(),
			Success:   hop.Success,
		}
		tr.Hops = append(tr.Hops, hop)
		//tr.HopsByIpAddress[strings.ReplaceAll(ip, ".", "-")] = hop
	}

	tracerouteStr, err := json.MarshalIndent(tr, "", "\t")
	if err != nil {
		return err
	}

	log.Infof("traceroute: %s", tracerouteStr)

	sender.EventPlatformEvent(tracerouteStr, epforwarder.EventTypeNetworkDevicesNetpath)
	return nil
}

func (c *Check) traceRouteV2(sender sender.Sender, hostHops [][]traceroute.TracerouteHop, hname string, destinationHost string) error {
	pathID := uuid.New()

	agentHostname := hname + "-TEST01"

	hops := hostHops[0]
	var prevhopID string
	for _, hop := range hops {
		hopIp := hop.AddressString()
		hopTTL := hop.TTL
		var hopID string
		if hopIp == "0.0.0.0" {
			hopID = fmt.Sprintf("hop:%s-%s-%d", agentHostname, destinationHost, hopTTL)
		} else {
			hopID = "ip:" + hopIp
		}
		hopRtt := hop.ElapsedTime.Seconds() * 10e3
		if hopTTL == 1 {
			prevhopID = "agent:" + agentHostname
		}
		tr := TracerouteV2{
			TracerouteSource: "netpath_integration",
			Strategy:         "hop_per_event",
			PathID:           pathID.String(),
			Timestamp:        time.Now().UnixMilli(),
			AgentHost:        agentHostname,
			DestinationHost:  destinationHost,

			// HOP
			HopTTL:      hopTTL,
			HopID:       hopID,
			HopIp:       hopIp,
			HopHostname: hop.HostOrAddressString(),
			HopRtt:      hopRtt,
			HopSuccess:  hop.Success,

			// Prev HOP
			PrevhopID: prevhopID,

			Message: "my-network-path",
			Team:    "network-device-monitoring",
		}
		tracerouteStr, err := json.MarshalIndent(tr, "", "\t")
		if err != nil {
			return err
		}

		log.Infof("traceroute: %s", string(tracerouteStr))

		sender.EventPlatformEvent(tracerouteStr, epforwarder.EventTypeNetworkDevicesNetpath)
		//tags := []string{
		//	"target_service:" + c.config.TargetService,
		//	"agent_host:" + agentHostname,
		//	"target:" + destinationHost,
		//	"hop_ip_address:" + hopIp,
		//	"hop_host:" + hop.HostOrAddressString(),
		//	"ttl:" + strconv.Itoa(hopTTL),
		//}
		//if prevhopID != "" {
		//	tags = append(tags, "prev_hop_ip_address:"+prevhopID)
		//	tags = append(tags, "prev_hop_host:"+prevhopID)
		//}
		//log.Infof("[netpath] tags: %s", tags)
		//sender.Gauge("netpath.hop.duration", hopRtt, "", CopyStrings(tags))
		//sender.Gauge("netpath.hop.record", float64(1), "", CopyStrings(tags))

		prevhopID = hopID
	}

	return nil
}

// Configure the CPU check
func (c *Check) Configure(integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	err := c.CommonConfigure(integrationConfigDigest, initConfig, data, source)
	if err != nil {
		return err
	}

	// Must be called before c.CommonConfigure
	c.BuildID(integrationConfigDigest, data, initConfig)

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
