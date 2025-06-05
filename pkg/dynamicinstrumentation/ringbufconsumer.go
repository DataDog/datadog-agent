// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dynamicinstrumentation

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/cilium/ebpf/ringbuf"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/eventparser"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ratelimiter"
)

func (goDI *GoDI) startRingbufferConsumer(rate float64) (func(), error) {
	r, err := ringbuf.NewReader(ditypes.EventsRingbuffer)
	if err != nil {
		return nil, fmt.Errorf("couldn't set up reader for ringbuffer: %w", err)
	}

	var record ringbuf.Record
	closeFunc := func() {
		r.Close()
	}

	// TODO: ensure rate limiters are removed once probes are removed
	rateLimiters := ratelimiter.NewMultiProbeRateLimiter(rate)
	rateLimiters.SetRate(ditypes.ConfigBPFProbeID, 0)

	go func() {
		for {
			err = r.ReadInto(&record)
			if err != nil {
				if errors.Is(err, ringbuf.ErrClosed) {
					return
				}
				log.Infof("couldn't read event off ringbuffer: %s", err.Error())
				continue
			}

			event, err := eventparser.ParseEvent(record.RawSample, rateLimiters)
			if err != nil {
				log.Trace(err)
				continue
			}
			if event == nil {
				continue
			}
			goDI.stats.PIDEventsCreatedCount[event.PID]++
			goDI.stats.ProbeEventsCreatedCount[event.ProbeID]++
			goDI.processEvent(event)
		}
	}()

	return closeFunc, nil
}
