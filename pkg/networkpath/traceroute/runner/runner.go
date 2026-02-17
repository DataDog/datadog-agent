// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package runner is the functionality for actually performing traceroutes
package runner

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"slices"
	"strings"
	"time"

	trcommon "github.com/DataDog/datadog-traceroute/common"
	tracerlog "github.com/DataDog/datadog-traceroute/log"
	"github.com/DataDog/datadog-traceroute/result"
	"github.com/DataDog/datadog-traceroute/traceroute"

	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	cloudprovidersnetwork "github.com/DataDog/datadog-agent/pkg/util/cloudproviders/network"
	"github.com/DataDog/datadog-agent/pkg/util/funcs"
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

func init() {
	tracerlog.SetLogger(tracerlog.Logger{
		Tracef:    log.Tracef,
		Infof:     log.Infof,
		Debugf:    log.Debugf,
		Warnf:     log.Warnf,
		Errorf:    log.Errorf,
		TraceFunc: log.TraceFunc,
	})
}

// Runner executes traceroutes
type Runner struct {
	gatewayLookup network.GatewayLookup
	nsIno         uint32
	networkID     func() string
	traceroute    *traceroute.Traceroute
}

// New initializes a new traceroute runner
func New(telemetryComp telemetryComponent.Component) (*Runner, error) {
	gatewayLookup, nsIno, err := createGatewayLookup(telemetryComp)
	if err != nil {
		log.Errorf("failed to create gateway lookup: %s", err.Error())
	}
	if gatewayLookup == nil {
		log.Warnf("gateway lookup is not enabled")
	}

	tracerouteInst := traceroute.NewTraceroute()
	return &Runner{
		gatewayLookup: gatewayLookup,
		nsIno:         nsIno,
		networkID: funcs.MemoizeNoError(func() string {
			nid, err := retryGetNetworkID()
			if err != nil {
				log.Errorf("failed to get network ID: %s", err.Error())
			}
			return nid
		}),
		traceroute: tracerouteInst,
	}, nil
}

// Start starts the traceroute runner
func (r *Runner) Start() {
	_ = r.networkID()
}

// Run wraps the implementation of traceroute
// so it can be called from the different OS implementations
//
// This code is experimental and will be replaced with a more
// complete implementation.
func (r *Runner) Run(ctx context.Context, cfg config.Config) (payload.NetworkPath, error) {
	defer tracerouteRunnerTelemetry.runs.Inc()

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

	params := traceroute.TracerouteParams{
		Hostname:              cfg.DestHostname,
		Port:                  int(cfg.DestPort),
		Protocol:              strings.ToLower(string(cfg.Protocol)),
		MinTTL:                trcommon.DefaultMinTTL,
		MaxTTL:                int(maxTTL),
		Delay:                 DefaultDelay,
		Timeout:               timeout,
		TCPMethod:             traceroute.TCPMethod(cfg.TCPMethod),
		WantV6:                false,
		ReverseDns:            cfg.ReverseDNS,
		CollectSourcePublicIP: true,
		UseWindowsDriver:      !cfg.DisableWindowsDriver,
		TracerouteQueries:     cfg.TracerouteQueries,
		E2eQueries:            cfg.E2eQueries,
	}

	results, err := r.traceroute.RunTraceroute(ctx, params)
	if err != nil {
		tracerouteRunnerTelemetry.failedRuns.Inc()
		return payload.NetworkPath{}, err
	}

	pathResult, err := r.processResults(results, cfg.Protocol, cfg.DestHostname, cfg.DestPort)
	if err != nil {
		tracerouteRunnerTelemetry.failedRuns.Inc()
		return payload.NetworkPath{}, err
	}
	log.Tracef("traceroute run results: %+v", pathResult)
	return pathResult, nil
}

func (r *Runner) processResults(res *result.Results, protocol payload.Protocol, destinationHost string, destinationPort uint16) (payload.NetworkPath, error) {
	if res == nil {
		return payload.NetworkPath{}, nil
	}

	traceroutePath := payload.NetworkPath{
		AgentVersion: version.AgentVersion,
		TestRunID:    res.TestRunID,
		Protocol:     protocol,
		Timestamp:    time.Now().UnixMilli(),
		Source: payload.NetworkPathSource{
			NetworkID: r.networkID(),
			PublicIP:  res.Source.PublicIP,
		},
		Destination: payload.NetworkPathDestination{
			Hostname: destinationHost,
			Port:     destinationPort,
		},
		Traceroute: payload.Traceroute{
			HopCount: payload.HopCountStats{
				Avg: res.Traceroute.HopCount.Avg,
				Min: res.Traceroute.HopCount.Min,
				Max: res.Traceroute.HopCount.Max,
			},
		},
		E2eProbe: payload.E2eProbe{
			RTTs:                 slices.Clone(res.E2eProbe.RTTs),
			PacketsSent:          res.E2eProbe.PacketsSent,
			PacketsReceived:      res.E2eProbe.PacketsReceived,
			PacketLossPercentage: res.E2eProbe.PacketLossPercentage,
			Jitter:               float64(res.E2eProbe.Jitter),
			RTT: payload.E2eProbeRttLatency{
				Avg: res.E2eProbe.RTT.Avg,
				Min: res.E2eProbe.RTT.Min,
				Max: res.E2eProbe.RTT.Max,
			},
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
	// Gateway lookup expect Source.Hostname and Destination.Hostname to be IPs
	if r.gatewayLookup != nil {
		src := util.AddressFromNetIP(net.ParseIP(traceroutePath.Source.Hostname))
		dst := util.AddressFromNetIP(net.ParseIP(traceroutePath.Destination.Hostname))

		traceroutePath.Source.Via = r.gatewayLookup.LookupWithIPs(src, dst, r.nsIno)
	}

	for _, run := range res.Traceroute.Runs {
		var hops []payload.TracerouteHop
		for _, hop := range run.Hops {
			hops = append(hops, payload.TracerouteHop{
				TTL:        hop.TTL,
				IPAddress:  hop.IPAddress,
				RTT:        hop.RTT,
				Reachable:  hop.Reachable,
				ReverseDNS: hop.ReverseDns,
			})
		}
		traceroutePath.Traceroute.Runs = append(traceroutePath.Traceroute.Runs, payload.TracerouteRun{
			RunID: run.RunID,
			Hops:  hops,
			Source: payload.TracerouteSource{
				IPAddress: run.Source.IPAddress,
				Port:      run.Source.Port,
			},
			Destination: payload.TracerouteDestination{
				IPAddress:  run.Destination.IPAddress,
				Port:       run.Destination.Port,
				ReverseDNS: run.Destination.ReverseDns,
			},
		})
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
