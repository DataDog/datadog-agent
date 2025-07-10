// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sack

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (p Params) validate() error {
	addr := p.Target.Addr()
	if !addr.IsValid() {
		return fmt.Errorf("SACK traceroute provided invalid IP address")
	}
	if addr.Is6() {
		return fmt.Errorf("SACK traceroute does not support IPv6")
	}
	return nil
}

// TODO MTU discovery, timestamps, etc? this will vary by platform
func setSockopts(_network, _address string, _c syscall.RawConn) error {
	return nil
}

func dialSackTCP(ctx context.Context, p Params) (net.Conn, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil, fmt.Errorf("dialTcp: expected a deadline")
	}

	d := net.Dialer{
		Timeout: p.HandshakeTimeout,
		Control: setSockopts,
	}
	target := p.Target.String()
	conn, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %w", target, err)
	}

	err = conn.SetDeadline(deadline)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to set deadline: %w", err)
	}
	return conn, err
}

type sackResult struct {
	LocalAddr netip.AddrPort
	Hops      []*common.ProbeResponse
}

func runSackTraceroute(ctx context.Context, p Params) (*sackResult, error) {
	err := p.validate()
	if err != nil {
		return nil, fmt.Errorf("invalid sack driver params: %w", err)
	}

	local, udpConn, err := common.LocalAddrForHost(p.Target.Addr().AsSlice(), p.Target.Port())
	if err != nil {
		return nil, fmt.Errorf("failed to get local addr: %w", err)
	}
	udpConn.Close()
	deadline := time.Now().Add(p.MaxTimeout())
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	// create the raw packet connection which watches for TCP/ICMP responses
	driver, err := newSackDriver(p, local.AddrPort().Addr())
	if err != nil {
		return nil, fmt.Errorf("failed to init sack driver: %w", err)
	}
	defer driver.Close()

	log.Debugf("sack traceroute dialing %s", p.Target)
	// now that the sackDriver is listening, dial the target. this is necessary
	// because sackDriver needs to watch the SYNACK to see if SACK is supported
	conn, err := dialSackTCP(ctx, p)
	if err != nil {
		// if we can't dial the remote (e.g. their server is not listening), we can't SACK traceroute,
		// but we could still SYN traceroute so return a NotSupportedError
		return nil, &NotSupportedError{
			Err: fmt.Errorf("sack traceroute failed to dial: %w", err),
		}
	}
	defer conn.Close()

	// sanity check that the local addr is what we expect
	tcpAddr, ok := conn.LocalAddr().(*net.TCPAddr)
	if !ok {
		return nil, fmt.Errorf("sack traceroute failed to get local addr: %w", err)
	}
	if tcpAddr.AddrPort().Addr() != local.AddrPort().Addr() {
		return nil, fmt.Errorf("tcp conn negotiated a different local addr than expected: %s != %s", tcpAddr.AddrPort(), local.AddrPort())
	}

	log.Debugf("sack traceroute reading handshake %s", p.Target)

	err = driver.ReadHandshake(tcpAddr.AddrPort().Port())
	if err != nil {
		return nil, fmt.Errorf("sack traceroute failed to read handshake: %w", err)
	}
	log.Debugf("sack traceroute running traceroute %s", p.Target)

	// this actually runs the traceroute
	resp, err := common.TracerouteParallel(ctx, driver, p.ParallelParams)
	if err != nil {
		return nil, fmt.Errorf("sack traceroute failed: %w", err)
	}

	result := &sackResult{
		LocalAddr: local.AddrPort(),
		Hops:      resp,
	}
	return result, nil
}

// RunSackTraceroute fully executes a SACK traceroute using the given parameters
func RunSackTraceroute(ctx context.Context, p Params) (*common.Results, error) {
	sackResult, err := runSackTraceroute(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("sack traceroute failed: %w", err)
	}

	hops, err := common.ToHops(p.ParallelParams.TracerouteParams, sackResult.Hops)
	if err != nil {
		return nil, fmt.Errorf("sack traceroute ToHops failed: %w", err)
	}

	result := &common.Results{
		Source:     sackResult.LocalAddr.Addr().AsSlice(),
		SourcePort: sackResult.LocalAddr.Port(),
		Target:     p.Target.Addr().AsSlice(),
		DstPort:    p.Target.Port(),
		Hops:       hops,
		Tags:       []string{"tcp_method:sack"},
	}

	return result, nil
}
