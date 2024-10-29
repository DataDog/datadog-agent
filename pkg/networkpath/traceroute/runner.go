// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traceroute

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net"
	"os"
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/Datadog/dublin-traceroute/go/dublintraceroute/probes/probev4"
	"github.com/Datadog/dublin-traceroute/go/dublintraceroute/results"
	"github.com/vishvananda/netns"

	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/tcp"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// DefaultSourcePort defines the default source port
	DefaultSourcePort = 12345
	// DefaultDestPort defines the default destination port
	DefaultDestPort = 33434
	// DefaultNumPaths defines the default number of paths
	DefaultNumPaths = 1
	// DefaultMinTTL defines the default minimum TTL
	DefaultMinTTL = 1
	// DefaultDelay defines the default delay
	DefaultDelay = 50 //msec
	// DefaultOutputFormat defines the default output format
	DefaultOutputFormat = "json"

	tracerouteRunnerModuleName = "traceroute_runner__"
)

// Telemetry
var tracerouteRunnerTelemetry = struct {
	runs                *telemetry.StatCounterWrapper
	failedRuns          *telemetry.StatCounterWrapper
	reverseDNSTimetouts *telemetry.StatCounterWrapper
}{
	telemetry.NewStatCounterWrapper(tracerouteRunnerModuleName, "runs", []string{}, "Counter measuring the number of traceroutes run"),
	telemetry.NewStatCounterWrapper(tracerouteRunnerModuleName, "failed_runs", []string{}, "Counter measuring the number of traceroute run failures"),
	telemetry.NewStatCounterWrapper(tracerouteRunnerModuleName, "reverse_dns_timeouts", []string{}, "Counter measuring the number of traceroute reverse DNS timeouts"),
}

// Runner executes traceroutes
type Runner struct {
	gatewayLookup network.GatewayLookup
	nsIno         uint32
	networkID     string
}

// NewRunner initializes a new traceroute runner
func NewRunner(telemetryComp telemetryComponent.Component) (*Runner, error) {
	var err error
	var networkID string
	if ec2.IsRunningOn(context.TODO()) {
		networkID, err = cloudproviders.GetNetworkID(context.Background())
		if err != nil {
			log.Errorf("failed to get network ID: %s", err.Error())
		}
	}

	gatewayLookup, nsIno, err := createGatewayLookup(telemetryComp)
	if err != nil {
		log.Errorf("failed to create gateway lookup: %s", err.Error())
	}
	if gatewayLookup == nil {
		log.Warnf("gateway lookup is not enabled")
	}

	return &Runner{
		gatewayLookup: gatewayLookup,
		nsIno:         nsIno,
		networkID:     networkID,
	}, nil
}

// RunTraceroute wraps the implementation of traceroute
// so it can be called from the different OS implementations
//
// This code is experimental and will be replaced with a more
// complete implementation.
func (r *Runner) RunTraceroute(ctx context.Context, cfg Config) (payload.NetworkPath, error) {
	defer tracerouteRunnerTelemetry.runs.Inc()
	dests, err := net.DefaultResolver.LookupIP(ctx, "ip4", cfg.DestHostname)
	if err != nil || len(dests) == 0 {
		tracerouteRunnerTelemetry.failedRuns.Inc()
		return payload.NetworkPath{}, fmt.Errorf("cannot resolve %s: %v", cfg.DestHostname, err)
	}

	//TODO: should we get smarter about IP address resolution?
	// if it's a hostname, perhaps we could run multiple traces
	// for each of the different IPs it resolves to up to a threshold?
	// use first resolved IP for now
	dest := dests[0]

	maxTTL := cfg.MaxTTL
	if maxTTL == 0 {
		maxTTL = setup.DefaultNetworkPathMaxTTL
	}

	var timeout time.Duration
	if cfg.Timeout == 0 {
		timeout = setup.DefaultNetworkPathTimeout * time.Duration(maxTTL) * time.Millisecond
	} else {
		timeout = cfg.Timeout
	}

	hname, err := hostname.Get(ctx)
	if err != nil {
		tracerouteRunnerTelemetry.failedRuns.Inc()
		return payload.NetworkPath{}, err
	}

	var pathResult payload.NetworkPath
	var protocol = cfg.Protocol

	// default to UDP if protocol
	// is not set
	if protocol == "" {
		protocol = payload.ProtocolUDP
	}
	switch protocol {
	case payload.ProtocolTCP:
		log.Tracef("Running TCP traceroute for: %+v", cfg)
		pathResult, err = r.runTCP(cfg, hname, dest, maxTTL, timeout)
		if err != nil {
			tracerouteRunnerTelemetry.failedRuns.Inc()
			return payload.NetworkPath{}, err
		}
	case payload.ProtocolUDP:
		log.Tracef("Running UDP traceroute for: %+v", cfg)
		pathResult, err = r.runUDP(cfg, hname, dest, maxTTL, timeout)
		if err != nil {
			tracerouteRunnerTelemetry.failedRuns.Inc()
			return payload.NetworkPath{}, err
		}
	default:
		log.Errorf("Invalid protocol for: %+v", cfg)
		tracerouteRunnerTelemetry.failedRuns.Inc()
		return payload.NetworkPath{}, fmt.Errorf("failed to run traceroute, invalid protocol: %s", cfg.Protocol)
	}

	return pathResult, nil
}

func (r *Runner) runUDP(cfg Config, hname string, dest net.IP, maxTTL uint8, timeout time.Duration) (payload.NetworkPath, error) {
	destPort, srcPort, useSourcePort := getPorts(cfg.DestPort)

	dt := &probev4.UDPv4{
		Target:     dest,
		SrcPort:    srcPort,
		DstPort:    destPort,
		UseSrcPort: useSourcePort,
		NumPaths:   uint16(DefaultNumPaths),
		MinTTL:     uint8(DefaultMinTTL), // TODO: what's a good value?
		MaxTTL:     maxTTL,
		Delay:      time.Duration(DefaultDelay) * time.Millisecond, // TODO: what's a good value?
		Timeout:    timeout,                                        // TODO: what's a good value?
		BrokenNAT:  false,
	}

	results, err := dt.Traceroute()
	if err != nil {
		return payload.NetworkPath{}, fmt.Errorf("traceroute run failed: %s", err.Error())
	}

	pathResult, err := r.processUDPResults(results, hname, cfg.DestHostname, destPort, dest)
	if err != nil {
		return payload.NetworkPath{}, err
	}
	log.Tracef("UDP Results: %+v", pathResult)

	return pathResult, nil
}

func (r *Runner) runTCP(cfg Config, hname string, target net.IP, maxTTL uint8, timeout time.Duration) (payload.NetworkPath, error) {
	destPort := cfg.DestPort
	if destPort == 0 {
		destPort = 80 // TODO: is this the default we want?
	}

	tr := tcp.TCPv4{
		Target:   target,
		DestPort: destPort,
		NumPaths: 1,
		MinTTL:   uint8(DefaultMinTTL),
		MaxTTL:   maxTTL,
		Delay:    time.Duration(DefaultDelay) * time.Millisecond,
		Timeout:  timeout,
	}

	results, err := tr.TracerouteSequential()
	if err != nil {
		return payload.NetworkPath{}, err
	}

	pathResult, err := r.processTCPResults(results, hname, cfg.DestHostname, destPort, target)
	if err != nil {
		return payload.NetworkPath{}, err
	}
	log.Tracef("TCP Results: %+v", pathResult)

	return pathResult, nil
}

func (r *Runner) processTCPResults(res *tcp.Results, hname string, destinationHost string, destinationPort uint16, destinationIP net.IP) (payload.NetworkPath, error) {
	traceroutePath := payload.NetworkPath{
		AgentVersion: version.AgentVersion,
		PathtraceID:  payload.NewPathtraceID(),
		Protocol:     payload.ProtocolTCP,
		Timestamp:    time.Now().UnixMilli(),
		Source: payload.NetworkPathSource{
			Hostname:  hname,
			NetworkID: r.networkID,
		},
		Destination: payload.NetworkPathDestination{
			Hostname:  destinationHost,
			Port:      destinationPort,
			IPAddress: destinationIP.String(),
		},
	}

	// get hardware interface info
	//
	// TODO: using a gateway lookup may be a more performant
	// solution for getting the local addr to use
	// when sending traceroute packets in the TCP implementation
	// really we just need the router piece
	// might be worth also looking in to sharing a router between
	// the gateway lookup and here or exposing a local IP lookup
	// function
	if r.gatewayLookup != nil {
		src := util.AddressFromNetIP(res.Source)
		dst := util.AddressFromNetIP(res.Target)

		traceroutePath.Source.Via = r.gatewayLookup.LookupWithIPs(src, dst, r.nsIno)
	}

	for i, hop := range res.Hops {
		ttl := i + 1
		isReachable := false
		hopname := fmt.Sprintf("unknown_hop_%d", ttl)
		hostname := hopname

		if !hop.IP.Equal(net.IP{}) {
			isReachable = true
			hopname = hop.IP.String()
		}

		npHop := payload.NetworkPathHop{
			TTL:       ttl,
			IPAddress: hopname,
			Hostname:  hostname,
			RTT:       float64(hop.RTT.Microseconds()) / float64(1000),
			Reachable: isReachable,
		}
		traceroutePath.Hops = append(traceroutePath.Hops, npHop)
	}

	return traceroutePath, nil
}

func (r *Runner) processUDPResults(res *results.Results, hname string, destinationHost string, destinationPort uint16, destinationIP net.IP) (payload.NetworkPath, error) {
	type node struct {
		node  string
		probe *results.Probe
	}

	traceroutePath := payload.NetworkPath{
		AgentVersion: version.AgentVersion,
		PathtraceID:  payload.NewPathtraceID(),
		Protocol:     payload.ProtocolUDP,
		Timestamp:    time.Now().UnixMilli(),
		Source: payload.NetworkPathSource{
			Hostname:  hname,
			NetworkID: r.networkID,
		},
		Destination: payload.NetworkPathDestination{
			Hostname:  destinationHost,
			Port:      destinationPort,
			IPAddress: destinationIP.String(),
		},
	}

	flowIDs := make([]int, 0, len(res.Flows))
	for flowID := range res.Flows {
		flowIDs = append(flowIDs, int(flowID))
	}
	sort.Ints(flowIDs)

	for _, flowID := range flowIDs {
		hops := res.Flows[uint16(flowID)]
		if len(hops) == 0 {
			log.Tracef("No hops for flow ID %d", flowID)
			continue
		}
		var nodes []node
		// add first hop
		localAddr := hops[0].Sent.IP.SrcIP

		// get hardware interface info
		if r.gatewayLookup != nil {
			src := util.AddressFromNetIP(localAddr)
			dst := util.AddressFromNetIP(hops[0].Sent.IP.DstIP)

			traceroutePath.Source.Via = r.gatewayLookup.LookupWithIPs(src, dst, r.nsIno)
		}

		firstNodeName := localAddr.String()
		nodes = append(nodes, node{node: firstNodeName, probe: &hops[0]})

		// then add all the other hops
		for _, hop := range hops {
			hop := hop
			nodename := fmt.Sprintf("unknown_hop_%d", hop.Sent.IP.TTL)
			if hop.Received != nil {
				nodename = hop.Received.IP.SrcIP.String()
			}
			nodes = append(nodes, node{node: nodename, probe: &hop})

			if hop.IsLast {
				break
			}
		}
		// add edges
		if len(nodes) <= 1 {
			// no edges to add if there is only one node
			continue
		}

		// start at node 1. Each node back-references the previous one
		for idx := 1; idx < len(nodes); idx++ {
			if idx >= len(nodes) {
				// we are at the second-to-last node
				break
			}
			prev := nodes[idx-1]
			cur := nodes[idx]

			edgeLabel := ""
			if idx == 1 {
				edgeLabel += fmt.Sprintf(
					"srcport %d\ndstport %d",
					cur.probe.Sent.UDP.SrcPort,
					cur.probe.Sent.UDP.DstPort,
				)
			}
			if prev.probe.NATID != cur.probe.NATID {
				edgeLabel += "\nNAT detected"
			}
			edgeLabel += fmt.Sprintf("\n%d.%d ms", int(cur.probe.RttUsec/1000), int(cur.probe.RttUsec%1000))

			isReachable := cur.probe.Received != nil
			ip := cur.node
			durationMs := float64(cur.probe.RttUsec) / 1000

			hop := payload.NetworkPathHop{
				TTL:       idx,
				IPAddress: ip,
				RTT:       durationMs,
				Reachable: isReachable,
			}
			traceroutePath.Hops = append(traceroutePath.Hops, hop)
		}
	}

	return traceroutePath, nil
}

func getPorts(configDestPort uint16) (uint16, uint16, bool) {
	var destPort uint16
	var srcPort uint16
	var useSourcePort bool
	if configDestPort > 0 {
		// Fixed Destination Port
		destPort = configDestPort
		useSourcePort = true
	} else {
		// Random Destination Port
		destPort = DefaultDestPort + uint16(rand.Intn(30))
		useSourcePort = false
	}
	srcPort = DefaultSourcePort + uint16(rand.Intn(10000))
	return destPort, srcPort, useSourcePort
}

func createGatewayLookup(telemetryComp telemetryComponent.Component) (network.GatewayLookup, uint32, error) {
	rootNs, err := rootNsLookup()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to look up root network namespace: %w", err)
	}
	defer rootNs.Close()

	nsIno, err := kernel.GetInoForNs(rootNs)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get inode number: %w", err)
	}

	gatewayLookup := network.NewGatewayLookup(rootNsLookup, math.MaxUint32, telemetryComp)
	return gatewayLookup, nsIno, nil
}

func rootNsLookup() (netns.NsHandle, error) {
	return netns.GetFromPid(os.Getpid())
}
