// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package runner

import (
	"net"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/udp"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// runUDP runs a UDP traceroute for Windows using a traceroute that's built in
// to the agent.
func (r *Runner) runUDP(cfg config.Config, hname string, target net.IP, maxTTL uint8, timeout time.Duration) (payload.NetworkPath, error) {
	destPort := cfg.DestPort
	if destPort == 0 {
		destPort = 33434 // TODO: is this the default we want?
	}

	tr := udp.NewUDPv4(target, destPort, DefaultNumPaths, uint8(DefaultMinTTL), maxTTL, time.Duration(DefaultDelay)*time.Millisecond, timeout)
	results, err := tr.TracerouteSequential()
	if err != nil {
		return payload.NetworkPath{}, err
	}

	pathResult, err := r.processResults(results, payload.ProtocolUDP, hname, cfg.DestHostname, cfg.DestPort)
	if err != nil {
		return payload.NetworkPath{}, err
	}
	log.Tracef("UDP Results: %+v", pathResult)

	return pathResult, nil
}
