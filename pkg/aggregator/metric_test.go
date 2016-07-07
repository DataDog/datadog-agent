package aggregator

import (

	// stdlib
	"testing"

	// datadog
	"github.com/DataDog/datadog-agent/pkg/util"
)

func TestGaugeSampling(t *testing.T) {
	// Initialize a new Gauge
	mGauge := Gauge{}

	// Add samples
	mGauge.addSample(1, 50)
	mGauge.addSample(2, 55)

	value, timestamp := mGauge.flush()
	// the last sample is flushed
	util.AssertAlmostEqual(t, value, 2)
	util.AssertAlmostEqual(t, timestamp, 55)
}
