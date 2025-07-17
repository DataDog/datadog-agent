// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package runner is the functionality for actually performing traceroutes
package runner

import (
	"context"
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/icmp"
	"math/rand"
	"net"
	"net/netip"
	"slices"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/sack"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/tcp"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	cloudprovidersnetwork "github.com/DataDog/datadog-agent/pkg/util/cloudproviders/network"
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
	gatewayLookup   network.GatewayLookup
	nsIno           uint32
	networkID       string
	hostnameService hostname.Component
}

// New initializes a new traceroute runner
func New(telemetryComp telemetryComponent.Component, hostnameService hostname.Component) (*Runner, error) {
	networkID, err := retryGetNetworkID()
	if err != nil {
		log.Errorf("failed to get network ID: %s", err.Error())
	}

	gatewayLookup, nsIno, err := createGatewayLookup(telemetryComp)
	if err != nil {
		log.Errorf("failed to create gateway lookup: %s", err.Error())
	}
	if gatewayLookup == nil {
		log.Warnf("gateway lookup is not enabled")
	}

	return &Runner{
		gatewayLookup:   gatewayLookup,
		nsIno:           nsIno,
		networkID:       networkID,
		hostnameService: hostnameService,
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

	hname, err := r.hostnameService.Get(ctx)
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
	case payload.ProtocolICMP:
		log.Tracef("Running ICMP traceroute for: %+v", cfg)
		pathResult, err = r.runICMP(cfg, hname, dest, maxTTL, timeout)
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

func (r *Runner) runICMP(cfg config.Config, hname string, target net.IP, maxTTL uint8, timeout time.Duration) (payload.NetworkPath, error) {
	targetAddr, ok := netip.AddrFromSlice(target)
	if !ok {
		return payload.NetworkPath{}, fmt.Errorf("invalid target IP")
	}
	results, err := icmp.RunICMPTraceroute(context.TODO(), icmp.Params{
		Target: targetAddr,
		ParallelParams: common.TracerouteParallelParams{
			TracerouteParams: common.TracerouteParams{
				MinTTL:            DefaultMinTTL,
				MaxTTL:            maxTTL,
				TracerouteTimeout: timeout,
				PollFrequency:     100 * time.Millisecond,
				SendDelay:         10 * time.Millisecond,
			},
		},
	})
	if err != nil {
		return payload.NetworkPath{}, err
	}
	pathResult, err := r.processResults(results, payload.ProtocolICMP, hname, cfg.DestHostname, cfg.DestPort)
	if err != nil {
		return payload.NetworkPath{}, err
	}
	log.Tracef("TCP Results: %+v", pathResult)

	return pathResult, nil
}

func makeSackParams(target net.IP, targetPort uint16, maxTTL uint8, timeout time.Duration) (sack.Params, error) {
	targetAddr, ok := netip.AddrFromSlice(target)
	if !ok {
		return sack.Params{}, fmt.Errorf("invalid target IP")
	}
	parallelParams := common.TracerouteParallelParams{
		TracerouteParams: common.TracerouteParams{
			MinTTL:            DefaultMinTTL,
			MaxTTL:            maxTTL,
			TracerouteTimeout: timeout,
			PollFrequency:     100 * time.Millisecond,
			SendDelay:         10 * time.Millisecond,
		},
	}
	params := sack.Params{
		Target:           netip.AddrPortFrom(targetAddr, targetPort),
		HandshakeTimeout: timeout,
		FinTimeout:       500 * time.Second,
		ParallelParams:   parallelParams,
		LoosenICMPSrc:    true,
	}
	return params, nil
}

var sackFallbackLimit = log.NewLogLimit(10, 5*time.Minute)

type tracerouteImpl func() (*common.Results, error)

func performTCPFallback(tcpMethod payload.TCPMethod, doSyn, doSack, doSynSocket tracerouteImpl) (*common.Results, error) {
	if tcpMethod == "" {
		tcpMethod = payload.TCPDefaultMethod
	}
	switch tcpMethod {
	case payload.TCPConfigSYN:
		return doSyn()
	case payload.TCPConfigSACK:
		return doSack()
	case payload.TCPConfigSYNSocket:
		return doSynSocket()
	case payload.TCPConfigPreferSACK:
		results, err := doSack()
		var sackNotSupportedErr *sack.NotSupportedError
		if errors.As(err, &sackNotSupportedErr) {
			if sackFallbackLimit.ShouldLog() {
				log.Infof("SACK traceroute not supported, falling back to SYN: %s", err)
			}
			return doSyn()
		}
		if err != nil {
			return nil, fmt.Errorf("SACK traceroute failed fatally, not falling back: %w", err)
		}
		return results, nil
	default:
		return nil, fmt.Errorf("unexpected TCPMethod: %s", tcpMethod)
	}
}

func (r *Runner) runTCP(cfg config.Config, hname string, target net.IP, maxTTL uint8, timeout time.Duration) (payload.NetworkPath, error) {
	destPort := cfg.DestPort
	if destPort == 0 {
		destPort = 80 // TODO: is this the default we want?
	}

	doSyn := func() (*common.Results, error) {
		tr := tcp.NewTCPv4(target, destPort, DefaultNumPaths, DefaultMinTTL, maxTTL, time.Duration(DefaultDelay)*time.Millisecond, timeout, cfg.TCPSynParisTracerouteMode)
		return tr.TracerouteSequential()
	}
	doSack := func() (*common.Results, error) {
		params, err := makeSackParams(target, destPort, maxTTL, timeout)
		if err != nil {
			return nil, fmt.Errorf("failed to make sack params: %w", err)
		}
		return sack.RunSackTraceroute(context.TODO(), params)
	}
	doSynSocket := func() (*common.Results, error) {
		tr := tcp.NewTCPv4(target, destPort, DefaultNumPaths, DefaultMinTTL, maxTTL, time.Duration(DefaultDelay)*time.Millisecond, timeout, cfg.TCPSynParisTracerouteMode)
		return tr.TracerouteSequentialSocket()
	}

	results, err := performTCPFallback(cfg.TCPMethod, doSyn, doSack, doSynSocket)
	if err != nil {
		return payload.NetworkPath{}, err
	}

	pathResult, err := r.processResults(results, payload.ProtocolTCP, hname, cfg.DestHostname, cfg.DestPort)
	if err != nil {
		return payload.NetworkPath{}, err
	}
	log.Tracef("TCP Results: %+v", pathResult)

	return pathResult, nil
}

func (r *Runner) processResults(res *common.Results, protocol payload.Protocol, hname string, destinationHost string, destinationPort uint16) (payload.NetworkPath, error) {
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
			Port:      destinationPort,
			IPAddress: res.Target.String(),
		},
		Tags: slices.Clone(res.Tags),
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

// retryGetNetworkID attempts to get the network ID from the cloud provider or config with a few retries
// as the endpoint is sometimes unavailable during host startup
func retryGetNetworkID() (string, error) {
	const maxRetries = 4
	var err error
	var networkID string
	for attempt := 1; attempt <= maxRetries; attempt++ {
		networkID, err = cloudprovidersnetwork.GetNetworkID(context.Background())
		if err == nil {
			return networkID, nil
		}
		log.Debugf(
			"failed to fetch network ID (attempt %d/%d): %s",
			attempt,
			maxRetries,
			err,
		)
		if attempt < maxRetries {
			time.Sleep(time.Duration(250*attempt) * time.Millisecond)
		}
	}
	return "", fmt.Errorf("failed to get network ID after %d attempts: %w", maxRetries, err)
}
