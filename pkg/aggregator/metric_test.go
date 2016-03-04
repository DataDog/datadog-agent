package aggregator

import (

	// stdlib
	"testing"

	// datadog
	"github.com/DataDog/datadog-agent/pkg/util"
)

func TestGaugeSampling(t *testing.T) {
	// Initialize a new Gauge
	mGauge := Gauge{1}

	// Previous value is overriden
	mGauge.addSample(2)
	util.AssertAlmostEqual(t, mGauge.gauge, 2)
}
