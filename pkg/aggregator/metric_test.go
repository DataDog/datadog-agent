package aggregator

import (

	// stdlib
	"testing"

	// 3p
	"github.com/stretchr/testify/assert"
)

const epsilon = 0.1

func TestGaugeSampling(t *testing.T) {
	// Initialize a new Gauge
	mGauge := Gauge{}

	// Add samples
	mGauge.addSample(1, 50)
	mGauge.addSample(2, 55)

	value, timestamp := mGauge.flush()
	// the last sample is flushed
	assert.InEpsilon(t, 2, value, epsilon)
	assert.EqualValues(t, timestamp, 55)
}

func TestRateSampling(t *testing.T) {
	// Initialize rates
	mRate1 := Rate{}
	mRate2 := Rate{}

	// Add samples
	mRate1.addSample(1, 50)
	mRate1.addSample(2, 55)
	mRate2.addSample(1, 50)

	// First rate
	value, timestamp, err := mRate1.flush()
	assert.InEpsilon(t, 0.2, value, epsilon)
	assert.EqualValues(t, timestamp, 55)
	assert.Nil(t, err)

	// Second rate (should return error)
	_, _, err = mRate2.flush()
	assert.NotNil(t, err)
}

func TestRateSamplingMultipleSamplesInSameFlush(t *testing.T) {
	// Initialize rate
	mRate := Rate{}

	// Add samples
	mRate.addSample(1, 50)
	mRate.addSample(2, 55)
	mRate.addSample(4, 60)

	// Should compute rate based on the last 2 samples
	value, timestamp, err := mRate.flush()
	assert.InEpsilon(t, 2./5., value, epsilon)
	assert.EqualValues(t, timestamp, 60)
	assert.Nil(t, err)
}

func TestRateSamplingNoSampleForOneFlush(t *testing.T) {
	// Initialize rate
	mRate := Rate{}

	// Add samples
	mRate.addSample(1, 50)
	mRate.addSample(2, 55)

	// First flush: no error
	_, _, err := mRate.flush()
	assert.Nil(t, err)

	// Second flush w/o sample: error
	_, _, err = mRate.flush()
	assert.NotNil(t, err)

	// Third flush w/ sample
	mRate.addSample(4, 60)
	// Should compute rate based on the last 2 samples
	value, timestamp, err := mRate.flush()
	assert.InEpsilon(t, 2./5., value, epsilon)
	assert.EqualValues(t, timestamp, 60)
	assert.Nil(t, err)
}
