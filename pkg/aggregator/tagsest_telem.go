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

// updateHugeSketches huge and almost-huge series in the given value
func (t *tagsetTelemetry) updateHugeSketchesTelemetry(sketches *metrics.SketchSeriesList) {
	counts := make([]uint64, t.size)
	var found bool

	for _, s := range *sketches {
		tags := uint64(len(s.Tags))
		for i, thresh := range t.sizeThresholds {
			if tags > thresh {
				counts[i]++
				found = true
			}
		}
	}

	if found {
		for i, count := range counts {
			if count > 0 {
				atomic.AddUint64(&t.hugeSketchesCount[i], count)
				t.tlmHugeSketches[i].Add(float64(count))
			}
		}
	}
}

// updateHugeSeriesTelemetry counts huge and almost-huge series in the given value
func (t *tagsetTelemetry) updateHugeSeriesTelemetry(series *metrics.Series) {
	counts := make([]uint64, t.size)
	var found bool

	for _, s := range *series {
		tags := uint64(len(s.Tags))
		for i, thresh := range t.sizeThresholds {
			if tags > thresh {
				counts[i]++
				found = true
			}
		}
	}

	if found {
		for i, count := range counts {
			if count > 0 {
				atomic.AddUint64(&t.hugeSeriesCount[i], count)
				t.tlmHugeSeries[i].Add(float64(count))
			}
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

func (t *tagsetTelemetry) reset() {
	for i := range t.sizeThresholds {
		atomic.StoreUint64(&t.hugeSeriesCount[i], uint64(0))
		atomic.StoreUint64(&t.hugeSketchesCount[i], uint64(0))
	}
}
