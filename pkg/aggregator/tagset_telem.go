// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"fmt"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

// The tagsetTelemetry struct handles telemetry for "large" tagsets.  For
// troubleshooting, we want to know how many of "large" tagsets we are handling
// for both timeseries and sketches.
type tagsetTelemetry struct {
	size int

	// sizeThresholds are thresholds over which we would like to count tagsets
	sizeThresholds []uint64

	// hugeSeriesCounts contains the total count of huge metric series, by
	// threshold. Access must be atomic.
	hugeSeriesCount []uint64

	// tlmHugeSeries is an array containing counters with the same values as
	// hugeSeriesCount.
	tlmHugeSeries []telemetry.Counter

	// hugeSketchesCount contains the total count of huge distributions, by
	// threshold. Access must be atomic.
	hugeSketchesCount []uint64

	// tlmHugeSketches is an array containing counters with the same values as
	// hugeSketchesCount.
	tlmHugeSketches []telemetry.Counter
}

func newTagsetTelemetry(thresholds []uint64) *tagsetTelemetry {
	size := len(thresholds)
	t := &tagsetTelemetry{
		size:              size,
		sizeThresholds:    thresholds,
		hugeSeriesCount:   make([]uint64, size, size),
		tlmHugeSeries:     make([]telemetry.Counter, size, size),
		hugeSketchesCount: make([]uint64, size, size),
		tlmHugeSketches:   make([]telemetry.Counter, size, size),
	}

	for i, thresh := range t.sizeThresholds {
		t.tlmHugeSeries[i] = telemetry.NewCounter("aggregator", fmt.Sprintf("series_tags_above_%d", thresh), nil, fmt.Sprintf("Count of timeseries with over %d tags", thresh))
		t.tlmHugeSketches[i] = telemetry.NewCounter("aggregator", fmt.Sprintf("distributions_tags_above_%d", thresh), nil, fmt.Sprintf("Count of distributions with over %d tags", thresh))
	}

	return t
}

// updateTelemetry implements common behavior fof the update*Telemetry methods.
func (t *tagsetTelemetry) updateTelemetry(tagsetSizes []uint64, atomicCounts []uint64, tlms []telemetry.Counter) {
	counts := make([]uint64, t.size)
	var found bool

	for _, tagsetSize := range tagsetSizes {
		for i, thresh := range t.sizeThresholds {
			if tagsetSize > thresh {
				counts[i]++
				found = true
			}
		}
	}

	if found {
		for i, count := range counts {
			if count > 0 {
				atomic.AddUint64(&atomicCounts[i], count)
				tlms[i].Add(float64(count))
			}
		}
	}
}

// updateHugeSketches huge and almost-huge series in the given value
func (t *tagsetTelemetry) updateHugeSketchesTelemetry(sketches *metrics.SketchSeriesList) {
	tagsetSizes := make([]uint64, len(*sketches))
	for i, s := range *sketches {
		tagsetSizes[i] = uint64(s.Tags.Len())
	}
	t.updateTelemetry(tagsetSizes, t.hugeSketchesCount, t.tlmHugeSketches)
}

// updateHugeSeriesTelemetry counts huge and almost-huge series in the given value
func (t *tagsetTelemetry) updateHugeSeriesTelemetry(series *metrics.Series) {
	tagsetSizes := make([]uint64, len(*series))
	for i, s := range *series {
		tagsetSizes[i] = uint64(s.Tags.Len())
	}
	t.updateTelemetry(tagsetSizes, t.hugeSeriesCount, t.tlmHugeSeries)
}

// updateHugeSerieTelemetry increments huge and almost-huge counters.
// Same as updateHugeSeriesTelemetry but for a single serie.
func (t *tagsetTelemetry) updateHugeSerieTelemetry(serie *metrics.Serie) {
	tagsetSize := uint64(serie.Tags.Len())
	for i, thresh := range t.sizeThresholds {
		if tagsetSize > thresh {
			atomic.AddUint64(&t.hugeSeriesCount[i], 1)
			t.tlmHugeSeries[i].Add(1)
		}
	}
}

func (t *tagsetTelemetry) exp() interface{} {
	rv := map[string]map[string]uint64{
		"Series":   {},
		"Sketches": {},
	}

	for i, thresh := range t.sizeThresholds {
		serieCount := atomic.LoadUint64(&t.hugeSeriesCount[i])
		distributionCount := atomic.LoadUint64(&t.hugeSketchesCount[i])
		rv["Series"][fmt.Sprintf("Above%d", thresh)] = serieCount
		rv["Sketches"][fmt.Sprintf("Above%d", thresh)] = distributionCount
	}

	return rv
}
