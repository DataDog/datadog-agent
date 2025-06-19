// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

import (
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ReceiveProbeNoPktError is returned when ReceiveProbe() didn't find anything new.
// This is normal if the RTT is long
type ReceiveProbeNoPktError struct {
	Err error
}

func (e *ReceiveProbeNoPktError) Error() string {
	return fmt.Sprintf("ReceiveProbe() didn't find any new packets: %s", e.Err)
}
func (e *ReceiveProbeNoPktError) Unwrap() error {
	return e.Err
}

// ErrPacketDidNotMatchTraceroute is returned when a packet does not match the traceroute,
// either because it is unrelated traffic or from a different traceroute instance.
var ErrPacketDidNotMatchTraceroute = &ReceiveProbeNoPktError{Err: fmt.Errorf("packet did not match the traceroute")}

// BadPacketError is a non-fatal error that occurs when a packet is malformed.
type BadPacketError struct {
	Err error
}

func (e *BadPacketError) Error() string {
	return fmt.Sprintf("Failed to parse packet: %s", e.Err)
}
func (e *BadPacketError) Unwrap() error {
	return e.Err
}

// ProbeResponse is the response of a single probe in a traceroute
type ProbeResponse struct {
	// TTL is the Time To Live of the probe that was originally sent
	TTL uint8
	// IP is the IP address of the responding host
	IP netip.Addr
	// RTT is the round-trip time of the probe
	RTT time.Duration
	// IsDest is true if the responding host is the destination
	IsDest bool
}

// TracerouteDriverInfo is metadata about a TracerouteDriver
type TracerouteDriverInfo struct {
	SupportsParallel bool
}

// TracerouteDriver is an implementation of traceroute send+receive of packets
type TracerouteDriver interface {
	// GetDriverInfo returns metadata about this driver
	GetDriverInfo() TracerouteDriverInfo
	// SendProbe sends a traceroute packet with a specific TTL
	SendProbe(ttl uint8) error
	// ReceiveProbe polls to get a traceroute response with a timeout.
	ReceiveProbe(timeout time.Duration) (*ProbeResponse, error)
}

// TracerouteParams are the parameters for a traceroute shared between serial and parallel
type TracerouteParams struct {
	// MinTTL is the TTL to start the traceroute at
	MinTTL uint8
	// MaxTTL is the TTL to end the traceroute at
	MaxTTL uint8
	// TracerouteTimeout is the maximum time to wait for a response
	TracerouteTimeout time.Duration
	// PollFrequency is how often to poll for a response
	PollFrequency time.Duration
	// SendDelay is the delay between sending probes (typically small)
	SendDelay time.Duration
}

func (p TracerouteParams) validate() error {
	if p.MinTTL > p.MaxTTL {
		return fmt.Errorf("min TTL must be less than or equal to max TTL")
	}
	if p.MinTTL < 1 {
		return fmt.Errorf("min TTL must be at least 1")
	}
	return nil
}

func (p TracerouteParams) validateProbe(probe *ProbeResponse) error {
	if probe == nil {
		return fmt.Errorf("ReceiveProbe() returned nil without an error (this indicates a bug in the TracerouteDriver)")
	}
	if probe.TTL < p.MinTTL || probe.TTL > p.MaxTTL {
		return fmt.Errorf("ReceiveProbe() received an invalid TTL: expected TTL in [%d, %d], got %d", p.MinTTL, p.MaxTTL, probe.TTL)
	}
	return nil
}

// ProbeCount returns the number of probes that will be sent
func (p TracerouteParams) ProbeCount() int {
	if p.MinTTL > p.MaxTTL {
		return 0
	}
	return int(p.MaxTTL) - int(p.MinTTL) + 1
}

// clipResults removes probes before the minTTL and after the destination
func clipResults(minTTL uint8, results []*ProbeResponse) []*ProbeResponse {
	destIdx := slices.IndexFunc(results, func(pr *ProbeResponse) bool {
		return pr != nil && pr.IsDest
	})
	// trim off anything after the destination
	if destIdx != -1 {
		results = slices.Clip(results[:destIdx+1])
	}

	return results[minTTL:]
}

// ToHops converts a list of ProbeResponses to a Results
// TODO remove this, and use a single type to represent results
func ToHops(p TracerouteParams, probes []*ProbeResponse) ([]*Hop, error) {
	if p.MinTTL != 1 {
		return nil, fmt.Errorf("ToHops: processResults() requires MinTTL == 1")
	}
	hops := make([]*Hop, len(probes))
	for i, probe := range probes {
		hops[i] = &Hop{}
		if probe != nil {
			hops[i].IP = probe.IP.AsSlice()
			hops[i].RTT = probe.RTT
			hops[i].IsDest = probe.IsDest
		}
	}
	return hops, nil
}

var badPktLimit = log.NewLogLimit(10, 5*time.Minute)

// CheckProbeRetryable returns whether ReceiveProbe failed due to a real error or just an irrelevant packet
func CheckProbeRetryable(funcName string, err error) bool {
	noPktErr := &ReceiveProbeNoPktError{}
	badPktErr := &BadPacketError{}
	if errors.As(err, &noPktErr) {
		return true
	} else if errors.As(err, &badPktErr) {
		if badPktLimit.ShouldLog() {
			log.Warnf("%s() saw a malformed packet: %s", funcName, err)
		}
		return true
	}
	return false
}
