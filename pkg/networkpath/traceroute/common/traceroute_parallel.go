// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TracerouteParallelParams are the parameters for TracerouteParallel
type TracerouteParallelParams struct {
	TracerouteParams
}

// MaxTimeout combines the timeout+probe delays into a total timeout for the traceroute
func (p TracerouteParallelParams) MaxTimeout() time.Duration {
	delaySum := p.SendDelay * time.Duration(p.ProbeCount())
	return p.TracerouteTimeout + delaySum
}

// TracerouteParallel runs a traceroute in parallel
func TracerouteParallel(ctx context.Context, t TracerouteDriver, p TracerouteParallelParams) ([]*ProbeResponse, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}

	info := t.GetDriverInfo()
	if !info.SupportsParallel {
		return nil, fmt.Errorf("tried to call TracerouteParallel on a TracerouteDriver that doesn't support parallel")
	}

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

	hasSent := make(chan struct{})

	// start a goroutine to SendProbe() in a loop
	g.Go(func() error {
		var sentOnce sync.Once
		defer sentOnce.Do(func() { close(hasSent) })

		for i := int(p.MinTTL); i <= int(p.MaxTTL); i++ {
			// leave if we got cancelled
			if writerCtx.Err() != nil {
				return nil
			}

			err := t.SendProbe(uint8(i))
			if err != nil {
				return fmt.Errorf("SendProbe() failed: %w", err)
			}
			sentOnce.Do(func() { close(hasSent) })

			time.Sleep(p.SendDelay)
		}
		return nil
	})

	g.Go(func() error {
		// Windows raw sockets don't let you read from them until you send something first.
		// If you try to read first, you will get WSAEINVAL. So wait until we send something
		<-hasSent
		for {
			// leave if we got cancelled, SendProbe() failed, etc
			// doesn't use writerCtx because when we find the destination, we writerCancel(), and we want to keep reading
			if groupCtx.Err() != nil {
				return nil
			}

			probe, err := t.ReceiveProbe(p.PollFrequency)
			if CheckProbeRetryable("ReceiveProbe", err) {
				continue
			} else if err != nil {
				return fmt.Errorf("ReceiveProbe() failed: %w", err)
			} else if err = p.validateProbe(probe); err != nil {
				return err
			}

			writeProbe(probe)
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

	return clipResults(p.MinTTL, results), nil
}
