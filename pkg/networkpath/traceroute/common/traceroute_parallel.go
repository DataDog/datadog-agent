// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ErrReceiveProbeNoPkt is returned when ReceiveProbe() didn't find anything new.
// This is normal if the RTT is long
var ErrReceiveProbeNoPkt = errors.New("ReceiveProbe() doesn't have new packets")

// BadPacketError is a non-fatal error that occurs when a packet is malformed.
type BadPacketError struct {
	Err error
}

func (e *BadPacketError) Error() string {
	return fmt.Sprintf("Failed to parse packet: %s", e.Err)
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
	// whether this driver uses a separate socket to read ICMP, and implements ReceiveICMPProbe
	UsesReceiveICMPProbe bool
}

// TracerouteDriver is an implementation of traceroute send+receive of packets
type TracerouteDriver interface {
	// GetDriverInfo returns metadata about this driver
	GetDriverInfo() TracerouteDriverInfo
	// SendProbe sends a traceroute packet with a specific TTL
	SendProbe(ttl uint8) error
	// ReceiveProbe polls to get a traceroute response with a timeout
	ReceiveProbe(timeout time.Duration) (*ProbeResponse, error)
	// ReceiveICMPProbe is identical to ReceiveProbe, just running in another goroutine
	ReceiveICMPProbe(timeout time.Duration) (*ProbeResponse, error)
}

// TracerouteParallelParams are the parameters for TracerouteParallel
type TracerouteParallelParams struct {
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

// ProbeCount returns the number of probes that will be sent
func (p TracerouteParallelParams) ProbeCount() int {
	if p.MinTTL > p.MaxTTL {
		return 0
	}
	return int(p.MaxTTL) - int(p.MinTTL) + 1
}

// MaxTimeout combines the timeout+probe delays into a total timeout for the traceroute
func (p TracerouteParallelParams) MaxTimeout() time.Duration {
	delaySum := p.SendDelay * time.Duration(p.ProbeCount())
	return p.TracerouteTimeout + delaySum
}

// TracerouteParallel runs a traceroute in parallel
func TracerouteParallel(ctx context.Context, t TracerouteDriver, p TracerouteParallelParams) ([]*ProbeResponse, error) {
	if p.MinTTL > p.MaxTTL {
		return nil, fmt.Errorf("min TTL must be less than or equal to max TTL")
	}
	if p.MinTTL < 1 {
		return nil, fmt.Errorf("min TTL must be at least 1")
	}
	info := t.GetDriverInfo()

	results := make([]*ProbeResponse, int(p.MaxTTL)+1)
	resultsMu := sync.Mutex{}
	writeProbe := func(probe *ProbeResponse) {
		log.Tracef("found probe %+v", probe)
		resultsMu.Lock()
		defer resultsMu.Unlock()
		previous := results[probe.TTL]

		// packets can get delivered twice - only use the first received probe to avoid overestimating RTT.
		// this is also important for SACK because SACK traceroute returns the lowest TTL found from ACKs
		shouldUpdate := previous == nil
		// but also just in case, never let ICMP responses "cover up" actual destination responses
		if previous != nil && !previous.IsDest && probe.IsDest {
			shouldUpdate = true
		}

		if shouldUpdate {
			results[probe.TTL] = probe
		}
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, p.MaxTimeout())
	defer cancel()

	g, groupCtx := errgroup.WithContext(timeoutCtx)
	writerCtx, writerCancel := context.WithCancel(groupCtx)
	defer writerCancel()

	// start a goroutine to SendProbe() in a loop
	g.Go(func() error {
		for i := int(p.MinTTL); i <= int(p.MaxTTL); i++ {
			// leave if we got cancelled
			select {
			case <-writerCtx.Done():
				return nil
			default:
			}

			err := t.SendProbe(uint8(i))
			if err != nil {
				return err
			}

			time.Sleep(p.SendDelay)
		}
		return nil
	})

	handleProbeFunc := func(funcName string, probeFunc func(timeout time.Duration) (*ProbeResponse, error)) {
		g.Go(func() error {
			for {
				// leave if we got cancelled, SendProbe() failed, etc
				select {
				// doesn't use writerCtx because even if we writerCancel(), we want to keep reading
				case <-groupCtx.Done():
					return nil
				default:
				}

				probe, err := probeFunc(p.PollFrequency)
				if CheckParallelRetryable(funcName, err) {
					continue
				} else if err != nil {
					return err
				}
				if probe == nil {
					return fmt.Errorf("%s() returned nil without an error (this indicates a bug in the TracerouteDriver)", funcName)
				}
				if probe.TTL < p.MinTTL || probe.TTL > p.MaxTTL {
					return fmt.Errorf("%s() received an invalid TTL (expected TTL in [%d, %d]): %d", funcName, p.MinTTL, p.MaxTTL, probe.TTL)
				}

				writeProbe(probe)
				// no need to send more probes if we found the destination
				if probe.IsDest {
					writerCancel()
				}
			}
		})
	}
	handleProbeFunc("ReceiveProbe", t.ReceiveProbe)
	if info.UsesReceiveICMPProbe {
		handleProbeFunc("ReceiveICMPProbe", t.ReceiveICMPProbe)
	}

	// check for an error from the goroutines
	err := g.Wait()
	if err != nil {
		return nil, err
	}

	// finally, if we got externally cancelled, report that
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// If we found the destination, trim the results array
	destIdx := slices.IndexFunc(results, func(pr *ProbeResponse) bool {
		return pr != nil && pr.IsDest
	})
	if destIdx != -1 {
		results = slices.Clip(results[:destIdx+1])
	}

	return results[p.MinTTL:], nil
}

// ToHops converts a list of ProbeResponses to a Results
// TODO remove this, and use a single type to represent results
func ToHops(p TracerouteParallelParams, probes []*ProbeResponse) ([]*Hop, error) {
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

// CheckParallelRetryable returns whether ReceiveProbe failed due to a real error or just an irrelevant packet
func CheckParallelRetryable(funcName string, err error) bool {
	badPktErr := &BadPacketError{}
	if errors.Is(err, ErrReceiveProbeNoPkt) {
		return true
	} else if errors.As(err, &badPktErr) {
		if badPktLimit.ShouldLog() {
			log.Warnf("%s() saw a malformed packet: %s", funcName, err)
		}
		return true
	}
	return false
}
