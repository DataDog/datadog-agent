// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package sack has selective ACK-based tracerouting logic
package sack

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/packets"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NotSupportedError means the target did not respond with the SACK Permitted
// TCP option, or we couldn't establish a TCP connection to begin with
type NotSupportedError struct {
	Err error
}

func (e *NotSupportedError) Error() string {
	return fmt.Sprintf("SACK not supported by the target: %s", e.Err)
}
func (e *NotSupportedError) Unwrap() error {
	return e.Err
}

// Params is the SACK traceroute parameters
type Params struct {
	// Target is the IP:port to traceroute
	Target netip.AddrPort
	// HandshakeTimeout is how long to wait for a handshake SYNACK to be seen
	HandshakeTimeout time.Duration
	// FinTimeout is how much extra time to allow for FIN to finish
	FinTimeout time.Duration
	// ParallelParams are the standard params for parallel traceroutes
	ParallelParams common.TracerouteParallelParams
	// LoosenICMPSrc disables checking the source IP/port in ICMP payloads when enabled.
	// Reason: Some environments don't properly translate the payload of an ICMP TTL exceeded
	// packet meaning you can't trust the source address to correspond to your own private IP.
	LoosenICMPSrc bool
}

// MaxTimeout returns the sum of all timeouts/delays for a SACK traceroute
func (p Params) MaxTimeout() time.Duration {
	return p.HandshakeTimeout + p.FinTimeout + p.ParallelParams.MaxTimeout()
}

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
	handle, err := packets.NewSourceSink(common.IPFamily(p.Target.Addr()))
	if err != nil {
		return nil, fmt.Errorf("SACK traceroute failed to make NewSourceSink: %w", err)
	}
	// we need to have another socket open by definition for SACK traceroute, so if that's not
	// allowed, this can't work
	if handle.MustClosePort {
		handle.Source.Close()
		handle.Sink.Close()
		return nil, fmt.Errorf("SACK traceroute is not supported on this platform")
	}

	driver, err := newSackDriver(p, local.AddrPort().Addr(), handle.Sink, handle.Source)
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
