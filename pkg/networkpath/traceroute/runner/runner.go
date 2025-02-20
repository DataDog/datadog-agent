// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package runner is the functionality for actually performing traceroutes
package runner

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net"
	"os"
	"time"

	"github.com/vishvananda/netns"

	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/tcp"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
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

// Runner executes traceroutes
type Runner struct {
	gatewayLookup network.GatewayLookup
	nsIno         uint32
	networkID     string
}

// New initializes a new traceroute runner
func New(telemetryComp telemetryComponent.Component) (*Runner, error) {
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
func (r *Runner) RunTraceroute(ctx context.Context, cfg config.Config) (payload.NetworkPath, error) {
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

func (r *Runner) runTCP(cfg config.Config, hname string, target net.IP, maxTTL uint8, timeout time.Duration) (payload.NetworkPath, error) {
	destPort := cfg.DestPort
	if destPort == 0 {
		destPort = 80 // TODO: is this the default we want?
	}

	tr := tcp.NewTCPv4(target, destPort, DefaultNumPaths, DefaultMinTTL, maxTTL, time.Duration(DefaultDelay)*time.Millisecond, timeout)

	results, err := tr.TracerouteSequential()
	if err != nil {
		return payload.NetworkPath{}, err
	}

	pathResult, err := r.processResults(results, payload.ProtocolTCP, hname, cfg.DestHostname)
	if err != nil {
		return payload.NetworkPath{}, err
	}
	log.Tracef("TCP Results: %+v", pathResult)

	return pathResult, nil
}

func (r *Runner) processResults(res *common.Results, protocol payload.Protocol, hname string, destinationHost string) (payload.NetworkPath, error) {
	if res == nil {
		return payload.NetworkPath{}, nil
	}

	traceroutePath := payload.NetworkPath{
		AgentVersion: version.AgentVersion,
		PathtraceID:  payload.NewPathtraceID(),
		Protocol:     protocol,
		Timestamp:    time.Now().UnixMilli(),
		Source: payload.NetworkPathSource{
			Hostname:  hname,
			NetworkID: r.networkID,
		},
		Destination: payload.NetworkPathDestination{
			Hostname:  destinationHost,
			Port:      res.DstPort,
			IPAddress: res.Target.String(),
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
			hostname = hopname // setting to ip address for now, reverse DNS lookup will override hostname field later
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
