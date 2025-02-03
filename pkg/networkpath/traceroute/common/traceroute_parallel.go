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
	"time"

	"golang.org/x/sync/errgroup"
)

// ErrReceiveProbeNoPkt is returned when ReceiveProbe() didn't find anything new.
// This is normal if the RTT is long
var ErrReceiveProbeNoPkt = errors.New("ReceiveProbe() doesn't have new packets")

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

// TracerouteDriver is an implementation of traceroute send+receive of packets
type TracerouteDriver interface {
	// Send a traceroute packet with a specific TTL
	SendProbe(ttl uint8) error
	// Poll to get a traceroute response with a timeout
	ReceiveProbe(timeout time.Duration) (*ProbeResponse, error)
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

// TracerouteParallel runs a traceroute in parallel
func TracerouteParallel(ctx context.Context, t TracerouteDriver, p TracerouteParallelParams) ([]*ProbeResponse, error) {
	if p.MinTTL > p.MaxTTL {
		return nil, fmt.Errorf("min TTL must be less than or equal to max TTL")
	}

	results := make([]*ProbeResponse, int(p.MaxTTL-p.MinTTL+1))

	delaySum := p.SendDelay * time.Duration(len(results))
	timeoutCtx, cancel := context.WithTimeout(ctx, p.TracerouteTimeout+delaySum)
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

	// poll for ReceiveProbe() in a loop and write it to results[]
	g.Go(func() error {
		for {
			// leave if we got cancelled, SendProbe() failed, etc
			select {
			// doesn't use writerCtx because even if we writerCancel(), we want to keep reading
			case <-groupCtx.Done():
				return nil
			default:
			}

			probe, err := t.ReceiveProbe(p.PollFrequency)
			if err == ErrReceiveProbeNoPkt {
				continue
			}
			if err != nil {
				return err
			}
			if probe == nil {
				return fmt.Errorf("ReceiveProbe() returned nil without an error (this indicates a bug in the TracerouteDriver)")
			}
			if probe.TTL < p.MinTTL || probe.TTL > p.MaxTTL {
				return fmt.Errorf("received an invalid TTL (expected TTL in [%d, %d]): %d", probe.TTL, p.MinTTL, p.MaxTTL)
			}

			results[probe.TTL-p.MinTTL] = probe
			// no need to send more probes if we found the destination
			if probe.IsDest {
				writerCancel()
			}
		}
	})

	// check for an error from the goroutines
	err := g.Wait()
	if err != nil {
		return nil, err
	}

	// finally, if we got externally cancelled, report that
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	destIdx := slices.IndexFunc(results, func(pr *ProbeResponse) bool {
		return pr != nil && pr.IsDest
	})
	// trim off anything after the destination
	if destIdx != -1 {
		results = slices.Clip(results[:destIdx+1])
	}

	return results, nil
}
