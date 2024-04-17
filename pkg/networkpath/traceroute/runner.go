// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traceroute

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/Datadog/dublin-traceroute/go/dublintraceroute/probes/probev4"
	"github.com/Datadog/dublin-traceroute/go/dublintraceroute/results"

	"github.com/google/uuid"
)

// TODO: are these good defaults?
const (
	DefaultSourcePort   = 12345
	DefaultDestPort     = 33434
	DefaultNumPaths     = 1
	DefaultMinTTL       = 1
	DefaultMaxTTL       = 30
	DefaultDelay        = 50 //msec
	DefaultReadTimeout  = 10 * time.Second
	DefaultOutputFormat = "json"

	tracerouteRunnerModuleName = "traceroute_runner__"
)

// Telemetry
var tracerouteRunnerTelemetry = struct {
	runs       *telemetry.StatCounterWrapper
	failedRuns *telemetry.StatCounterWrapper
}{
	telemetry.NewStatCounterWrapper(tracerouteRunnerModuleName, "runs", []string{}, "Counter measuring the number of traceroutes run"),
	telemetry.NewStatCounterWrapper(tracerouteRunnerModuleName, "failed_runs", []string{}, "Counter measuring the number of traceroute run failures"),
}

// NewRunner initializes a new traceroute runner
func NewRunner() (*Runner, error) {
	// TODO: we should probably cache this info
	// double check the underlying call doesn't
	// already do this
	networkID, err := ec2.GetNetworkID(context.Background())
	if err != nil {
		log.Errorf("failed to get network ID: %s", err.Error())
	}

	gatewayLookup := network.NewGatewayLookup()
	if gatewayLookup != nil {
		log.Warnf("gateway lookup is not enabled")
	}

	return &Runner{
		gatewayLookup: gatewayLookup,
		networkID:     networkID,
	}, nil
}

// RunTraceroute wraps the implementation of traceroute
// so it can be called from the different OS implementations
//
// This code is experimental and will be replaced with a more
// complete implementation.
func (r *Runner) RunTraceroute(ctx context.Context, cfg Config) (NetworkPath, error) {
	rawDest := cfg.DestHostname
	dests, err := net.DefaultResolver.LookupIP(ctx, "ip4", rawDest)
	if err != nil || len(dests) == 0 {
		return NetworkPath{}, fmt.Errorf("cannot resolve %s: %v", rawDest, err)
	}

	//TODO: should we get smarter about IP address resolution?
	// if it's a hostname, perhaps we could run multiple traces
	// for each of the different IPs it resolves to up to a threshold?
	// use first resolved IP for now
	dest := dests[0]

	destPort, srcPort, useSourcePort := getPorts(cfg.DestPort)

	maxTTL := cfg.MaxTTL
	if maxTTL == 0 {
		maxTTL = DefaultMaxTTL
	}

	var timeout time.Duration
	if cfg.TimeoutMs == 0 {
		timeout = DefaultReadTimeout
	} else {
		timeout = time.Duration(cfg.TimeoutMs) * time.Millisecond
	}

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

	log.Debugf("Traceroute UDPv4 probe config: %+v", dt)
	results, err := dt.Traceroute()
	if err != nil {
		tracerouteRunnerTelemetry.runs.Inc()
		tracerouteRunnerTelemetry.failedRuns.Inc()
		return NetworkPath{}, fmt.Errorf("traceroute run failed: %s", err.Error())
	}
	//log.Debugf("Raw results: %+v", results)

	hname, err := hostname.Get(ctx)
	if err != nil {
		return NetworkPath{}, err
	}

	pathResult, err := r.processResults(ctx, results, hname, rawDest, dest)
	if err != nil {
		return NetworkPath{}, err
	}
	log.Debugf("Processed Results: %+v", results)

	// TODO: better tagging
	tracerouteRunnerTelemetry.runs.Inc()
	return pathResult, nil
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

func (r *Runner) processResults(ctx context.Context, res *results.Results, hname string, destinationHost string, destinationIP net.IP) (NetworkPath, error) {
	type node struct {
		node  string
		probe *results.Probe
	}

	pathID := uuid.New().String()

	traceroutePath := NetworkPath{
		PathID:    pathID,
		Timestamp: time.Now().UnixMilli(),
		Source: NetworkPathSource{
			Hostname:  hname,
			NetworkID: r.networkID,
		},
		Destination: NetworkPathDestination{
			Hostname:  destinationHost,
			IPAddress: destinationIP.String(),
		},
	}

	for idx, probes := range res.Flows {
		log.Debugf("Flow idx: %d\n", idx)
		for probleIndex, probe := range probes {
			log.Debugf("%d - %d - %s\n", probleIndex, probe.Sent.IP.TTL, probe.Name)
		}
	}

	flowIDs := make([]int, 0, len(res.Flows))
	for flowID := range res.Flows {
		flowIDs = append(flowIDs, int(flowID))
	}
	sort.Ints(flowIDs)

	for _, flowID := range flowIDs {
		hops := res.Flows[uint16(flowID)]
		if len(hops) == 0 {
			log.Debugf("No hops for flow ID %d", flowID)
			continue
		}
		var nodes []node
		// add first hop
		localAddr := hops[0].Sent.IP.SrcIP

		// get hardware interface info
		if r.gatewayLookup != nil {
			src := util.AddressFromNetIP(localAddr)
			dst := util.AddressFromNetIP(hops[0].Sent.IP.DstIP)

			// TODO: how do I get namespace here?
			traceroutePath.Source.Via = r.gatewayLookup.LookupWithIPs(src, dst, 0)
		}

		firstNodeName := localAddr.String()
		nodes = append(nodes, node{node: firstNodeName, probe: &hops[0]})

		// then add all the other hops
		for _, hop := range hops {
			hop := hop
			nodename := fmt.Sprintf("unknown_hop_%d)", hop.Sent.IP.TTL)
			label := "*"
			hostname := ""
			if hop.Received != nil {
				nodename = hop.Received.IP.SrcIP.String()
				if hop.Name != nodename {
					hostname = "\n" + hop.Name
				}
				// MPLS labels
				mpls := ""
				if len(hop.Received.ICMP.MPLSLabels) > 0 {
					mpls = "MPLS labels: \n"
					for _, mplsLabel := range hop.Received.ICMP.MPLSLabels {
						mpls += fmt.Sprintf(" - %d, ttl: %d\n", mplsLabel.Label, mplsLabel.TTL)
					}
				}
				label = fmt.Sprintf("%s%s\n%s\n%s", nodename, hostname, hop.Received.ICMP.Description, mpls)
			}
			nodes = append(nodes, node{node: nodename, probe: &hop})

			if hop.IsLast {
				break
			}
			log.Debugf("Label: %s", label)
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

			isSuccess := cur.probe.Received != nil
			ip := cur.node
			durationMs := float64(cur.probe.RttUsec) / 1000

			hop := NetworkPathHop{
				TTL:       idx,
				IPAddress: ip,
				Hostname:  getHostname(cur.node),
				RTT:       durationMs,
				Success:   isSuccess,
			}
			traceroutePath.Hops = append(traceroutePath.Hops, hop)
		}
	}

	log.Debugf("Traceroute path metadata payload: %+v", traceroutePath)
	return traceroutePath, nil
}

func getHostname(ipAddr string) string {
	// TODO: this reverse lookup appears to have some standard timeout that is relatively
	// high. Consider switching to something where there is greater control.
	currHost := ""
	currHostList, _ := net.LookupAddr(ipAddr)
	log.Debugf("Reverse DNS List: %+v", currHostList)

	if len(currHostList) > 0 {
		// TODO: Reverse DNS: Do we need to handle cases with multiple DNS being returned?
		currHost = currHostList[0]
	} else {
		currHost = ipAddr
	}
	return currHost
}
