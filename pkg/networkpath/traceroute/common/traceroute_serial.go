// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TracerouteSerialParams are the parameters for TracerouteSerial
type TracerouteSerialParams struct {
	TracerouteParams
}

// TracerouteSerial runs a traceroute in serial. Sometimes this is necessary over TracerouteParallel
// because the driver doesn't support parallel.
func TracerouteSerial(ctx context.Context, t TracerouteDriver, p TracerouteSerialParams) ([]*ProbeResponse, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}

	results := make([]*ProbeResponse, int(p.MaxTTL)+1)
	for i := int(p.MinTTL); i <= int(p.MaxTTL); i++ {
		if ctx.Err() != nil {
			break
		}
		sendDelay := time.After(p.SendDelay)

		timeoutCtx, cancel := context.WithTimeout(ctx, p.TracerouteTimeout)
		defer cancel()

		err := t.SendProbe(uint8(i))
		if err != nil {
			return nil, fmt.Errorf("SendProbe() failed: %w", err)
		}

		var probe *ProbeResponse
		for probe == nil {
			if timeoutCtx.Err() != nil {
				break
			}

			probe, err = t.ReceiveProbe(p.PollFrequency)
			if CheckProbeRetryable("ReceiveProbe", err) {
				continue
			} else if err != nil {
				return nil, fmt.Errorf("ReceiveProbe() failed: %w", err)
			} else if err := p.validateProbe(probe); err != nil {
				return nil, err
			}
		}

		if probe != nil {
			log.Tracef("found probe %+v", probe)
			// if we found the destination, no need to keep going
			results[probe.TTL] = probe
			if probe.IsDest {
				break
			}
		}

		// wait for at least SendDelay to pass
		<-sendDelay
	}

	// if we got externally cancelled, report that
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	return clipResults(p.MinTTL, results), nil
}
