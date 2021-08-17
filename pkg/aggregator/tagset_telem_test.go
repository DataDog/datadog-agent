// +build test

package aggregator

import "sync/atomic"

// (only used in tests) reset the tagset telemetry to zeroes
func (t *tagsetTelemetry) reset() {
	for i := range t.sizeThresholds {
		atomic.StoreUint64(&t.hugeSeriesCount[i], uint64(0))
		atomic.StoreUint64(&t.hugeSketchesCount[i], uint64(0))
	}
}
