// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"fmt"

	"go.uber.org/atomic"

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
	// threshold.
	hugeSeriesCount []*atomic.Uint64

	// tlmHugeSeries is an array containing counters with the same values as
	// hugeSeriesCount.
	tlmHugeSeries []telemetry.Counter

	// hugeSketchesCount contains the total count of huge distributions, by
	// threshold.
	hugeSketchesCount []*atomic.Uint64

	// tlmHugeSketches is an array containing counters with the same values as
	// hugeSketchesCount.
	tlmHugeSketches []telemetry.Counter
}

func newTagsetTelemetry(thresholds []uint64) *tagsetTelemetry {
	size := len(thresholds)
	t := &tagsetTelemetry{
		size:              size,
		sizeThresholds:    thresholds,
		hugeSeriesCount:   make([]*atomic.Uint64, size),
		tlmHugeSeries:     make([]telemetry.Counter, size),
		hugeSketchesCount: make([]*atomic.Uint64, size),
		tlmHugeSketches:   make([]telemetry.Counter, size),
	}

	for i, thresh := range t.sizeThresholds {
		t.hugeSeriesCount[i] = atomic.NewUint64(0)
		t.tlmHugeSeries[i] = telemetry.NewCounter("aggregator", fmt.Sprintf("series_tags_above_%d", thresh), nil, fmt.Sprintf("Count of timeseries with over %d tags", thresh))
		t.hugeSketchesCount[i] = atomic.NewUint64(0)
		t.tlmHugeSketches[i] = telemetry.NewCounter("aggregator", fmt.Sprintf("distributions_tags_above_%d", thresh), nil, fmt.Sprintf("Count of distributions with over %d tags", thresh))
	}

	return t
}

// updateTelemetry implements common behavior fof the update*Telemetry methods.
func (t *tagsetTelemetry) updateTelemetry(tagsetSize uint64, atomicCounts []*atomic.Uint64, tlms []telemetry.Counter) {
	for i, thresh := range t.sizeThresholds {
		if tagsetSize > thresh {
			atomicCounts[i].Inc()
			tlms[i].Add(1)
		}
	}
}

// updateHugeSketches huge and almost-huge series in the given value
func (t *tagsetTelemetry) updateHugeSketchesTelemetry(sketches *metrics.SketchSeries) {
	tagsetSize := uint64(sketches.Tags.Len())
	t.updateTelemetry(tagsetSize, t.hugeSketchesCount, t.tlmHugeSketches)
}

// updateHugeSerieTelemetry increments huge and almost-huge counters.
// Same as updateHugeSeriesTelemetry but for a single serie.
func (t *tagsetTelemetry) updateHugeSerieTelemetry(serie *metrics.Serie) {
	tagsetSize := uint64(serie.Tags.Len())
	t.updateTelemetry(tagsetSize, t.hugeSeriesCount, t.tlmHugeSeries)
}

func (t *tagsetTelemetry) exp() interface{} {
	rv := map[string]map[string]uint64{
		"Series":   {},
		"Sketches": {},
	}

	for i, thresh := range t.sizeThresholds {
		serieCount := t.hugeSeriesCount[i].Load()
		distributionCount := t.hugeSketchesCount[i].Load()
		rv["Series"][fmt.Sprintf("Above%d", thresh)] = serieCount
		rv["Sketches"][fmt.Sprintf("Above%d", thresh)] = distributionCount
	}

	return rv
}
