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
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/netpath/dublintraceroute"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/netpath/dublintraceroute/probes/probev4"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/netpath/traceroute"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/shirou/gopsutil/v3/cpu"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

// Program constants and default values
const (
	ProgramName         = "Dublin Traceroute"
	ProgramVersion      = "v0.2"
	ProgramAuthorName   = "Andrea Barberio"
	ProgramAuthorInfo   = "https://insomniac.slackware.it"
	DefaultSourcePort   = 12345
	DefaultDestPort     = 33434
	DefaultNumPaths     = 10
	DefaultMinTTL       = 1
	DefaultMaxTTL       = 30
	DefaultDelay        = 50 //msec
	DefaultReadTimeout  = 3 * time.Second
	DefaultOutputFormat = "json"
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
	rawTarget := c.config.Hostname
	target, err := resolve(rawTarget, false)
	if err != nil {
		return fmt.Errorf("Cannot resolve %s: %v", rawTarget, err)
	}

	var dt dublintraceroute.DublinTraceroute
	dt = &probev4.UDPv4{
		Target:     target,
		SrcPort:    uint16(DefaultSourcePort),
		DstPort:    uint16(DefaultDestPort),
		UseSrcPort: false,
		NumPaths:   uint16(DefaultNumPaths),
		MinTTL:     uint8(DefaultMinTTL),
		MaxTTL:     uint8(DefaultMaxTTL),
		Delay:      time.Duration(DefaultDelay) * time.Millisecond,
		Timeout:    DefaultReadTimeout,
		BrokenNAT:  false,
	}
	results, err := dt.Traceroute()
	if err != nil {
		return fmt.Errorf("Traceroute() failed: %v", err)
	}
	log.Warnf("results: %+v", results)

	return nil
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

	err = c.traceRouteV2(sender, hostHops, hname, destinationHost)
	if err != nil {
		return err
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
		tr.HopsByIpAddress[strings.ReplaceAll(ip, ".", "-")] = hop
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
	hops := hostHops[0]
	var prevHop traceroute.TracerouteHop
	for _, hop := range hops {
		ip := hop.AddressString()
		durationMs := hop.ElapsedTime.Seconds() * 10e3
		tr := TracerouteV2{
			TracerouteSource: "netpath_integration",
			Timestamp:        time.Now().UnixMilli(),
			AgentHost:        hname,
			DestinationHost:  destinationHost,
			TTL:              hop.TTL,
			IpAddress:        ip,
			Host:             hop.HostOrAddressString(),
			Duration:         durationMs,
			Success:          hop.Success,
		}
		tracerouteStr, err := json.MarshalIndent(tr, "", "\t")
		if err != nil {
			return err
		}

		log.Infof("traceroute: %s", tracerouteStr)

		sender.EventPlatformEvent(tracerouteStr, epforwarder.EventTypeNetworkDevicesNetpath)
		tags := []string{
			"target_service:" + c.config.TargetService,
			"agent_host:" + hname,
			"target:" + destinationHost,
			"hop_ip_address:" + ip,
			"hop_host:" + hop.HostOrAddressString(),
			"ttl:" + strconv.Itoa(hop.TTL),
		}
		if prevHop.TTL > 0 {
			prevIp := prevHop.AddressString()
			tags = append(tags, "prev_hop_ip_address:"+prevIp)
			tags = append(tags, "prev_hop_host:"+prevHop.HostOrAddressString())
		}
		log.Infof("[netpath] tags: %s", tags)
		sender.Gauge("netpath.hop.duration", durationMs, "", CopyStrings(tags))
		sender.Gauge("netpath.hop.record", float64(1), "", CopyStrings(tags))

		prevHop = hop
	}

	return nil
}

// resolve returns the first IP address for the given host. If `wantV6` is true,
// it will return the first IPv6 address, or nil if none. Similarly for IPv4
// when `wantV6` is false.
// If the host is already an IP address, such IP address will be returned. If
// `wantV6` is true but no IPv6 address is found, it will return an error.
// Similarly for IPv4 when `wantV6` is false.
func resolve(host string, wantV6 bool) (net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if wantV6 && ip.To4() != nil {
			return nil, errors.New("Wanted an IPv6 address but got an IPv4 address")
		} else if !wantV6 && ip.To4() == nil {
			return nil, errors.New("Wanted an IPv4 address but got an IPv6 address")
		}
		return ip, nil
	}
	ipaddrs, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	var ret net.IP
	for _, ipaddr := range ipaddrs {
		if wantV6 && ipaddr.To4() == nil {
			ret = ipaddr
			break
		} else if !wantV6 && ipaddr.To4() != nil {
			ret = ipaddr
		}
	}
	if ret == nil {
		return nil, errors.New("No IP address of the requested type was found")
	}
	return ret, nil
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
