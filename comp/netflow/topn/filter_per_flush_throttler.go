// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package topn defines business logic for filtering NetFlow records to the Top "N" occurrences.
package topn

import (
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
)

type throttler struct {
	flushConfig common.FlushConfig
	logger      log.Component

	k                       int64
	flushesPerPeriod        int64
	numFlushesWithExtraRows int64
}

func newThrottler(n int64, flushConfig common.FlushConfig, logger log.Component) *throttler {
	flushesPerPeriod := int64(flushConfig.FlowCollectionDuration / flushConfig.FlushTickFrequency)
	k := n / flushesPerPeriod
	extraRows := n % flushesPerPeriod

	return &throttler{
		flushConfig: flushConfig,
		logger:      logger,

		k:                       k,
		flushesPerPeriod:        flushesPerPeriod,
		numFlushesWithExtraRows: extraRows,
	}
}

// GetNumRowsToFlushFor takes in the flush context and returns the number of rows that should be flushed according to TopN
// heuristics. It handles modular rounding of # of flows to publish for a given period so that we publish
// exactly N records. It will also handle when the flush context states that multiple flushes are being
// published in one run.
//
// If we had top N = 25, and 13 ticks per flush period, then the distribution for "k" that we'd land on is:
//
//	[ a, b, c, d, e, f, g, h, i, j, k, l, m ]<- "flush bucket"
//	  2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 1  <- # of flows published for bucket
func (s *throttler) GetNumRowsToFlushFor(ctx common.FlushContext) int {
	// convert the flush time to a frame of reference of the flush collection duration
	// this lets us more deterministically calculate the # of periods we're covering in this flush
	flushTimeNormalized := ctx.FlushTime.UnixNano() % int64(s.flushConfig.FlowCollectionDuration)
	// determine the bucket the flush is occurring in, range is [0, n) where n is the # of ticks per flush period
	flushGenerationIndex := flushTimeNormalized / int64(s.flushConfig.FlushTickFrequency)

	// clamp to the last generation if we're over for some reason
	if flushGenerationIndex >= s.flushesPerPeriod {
		flushGenerationIndex = s.flushesPerPeriod - 1
	}

	type interval struct {
		startInclusive int64
		endInclusive   int64
	}
	var intervals []interval

	// we know the last bucket we're publishing, now we need to determine the first one.
	// we get the number of flushes in the ctx so we'll use that:
	earliestFlushIndex := flushGenerationIndex + 1 - ctx.NumFlushes

	if earliestFlushIndex < 0 {
		// it wraps around the 0th index to be the back half, break it into two different intervals so it's easier to calculate
		intervals = append(intervals, interval{
			startInclusive: 0,
			endInclusive:   flushGenerationIndex,
		}, interval{
			startInclusive: earliestFlushIndex + s.flushesPerPeriod,
			endInclusive:   s.flushesPerPeriod - 1,
		})
	} else {
		intervals = append(intervals, interval{
			startInclusive: earliestFlushIndex,
			endInclusive:   flushGenerationIndex,
		})
	}

	numRows := 0
	for _, interval := range intervals {
		numRows += s.calculateNumFlowsOverInterval(interval)
	}
	return numRows
}

func (s *throttler) calculateNumFlowsOverInterval(flushInterval struct {
	startInclusive int64
	endInclusive   int64
}) int {
	if flushInterval.startInclusive > flushInterval.endInclusive {
		// illegal state!
		_ = s.logger.Warn("top-n throttling is in an illegal state, startInclusive is greater than endInclusive %+v", flushInterval)
		return 0
	}

	total := s.k * (1 + flushInterval.endInclusive - flushInterval.startInclusive)
	if s.numFlushesWithExtraRows-1 >= flushInterval.startInclusive {
		end := s.numFlushesWithExtraRows - 1
		if end > flushInterval.endInclusive {
			end = flushInterval.endInclusive
		}

		total += end - flushInterval.startInclusive + 1
	}

	return int(total)
}
