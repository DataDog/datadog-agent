// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build unix

package runner

import (
	"fmt"
	"net"
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/Datadog/dublin-traceroute/go/dublintraceroute/probes/probev4"
	"github.com/Datadog/dublin-traceroute/go/dublintraceroute/results"
)

// runUDP runs a UDP traceroute using the Dublin Traceroute library.
func (r *Runner) runUDP(cfg config.Config, hname string, dest net.IP, maxTTL uint8, timeout time.Duration) (payload.NetworkPath, error) {
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

	pathResult, err := r.processDublinResults(results, hname, cfg.DestHostname, cfg.DestPort, dest)
	if err != nil {
		return payload.NetworkPath{}, err
	}
	log.Tracef("UDP Results: %+v", pathResult)

	return pathResult, nil
}

func (r *Runner) processDublinResults(res *results.Results, hname string, destinationHost string, destinationPort uint16, destinationIP net.IP) (payload.NetworkPath, error) {
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
