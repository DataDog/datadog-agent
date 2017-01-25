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

	series, _ := mGauge.flush(60)
	// the last sample is flushed
	assert.Len(t, series, 1)
	assert.Len(t, series[0].Points, 1)
	assert.InEpsilon(t, 2, series[0].Points[0][1], epsilon)
	assert.EqualValues(t, series[0].Points[0][0], 60)
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
	series, err := mRate1.flush(60)
	assert.Nil(t, err)
	assert.Len(t, series, 1)
	assert.Len(t, series[0].Points, 1)
	assert.InEpsilon(t, 0.2, series[0].Points[0][1], epsilon)
	assert.EqualValues(t, series[0].Points[0][0], 55)

	// Second rate (should return error)
	_, err = mRate2.flush(60)
	assert.NotNil(t, err)
}

func TestRateSamplingMultipleSamplesInSameFlush(t *testing.T) {
	// Initialize rate
	mRate := Rate{}

	// Add samples
	mRate.addSample(1, 50)
	mRate.addSample(2, 55)
	mRate.addSample(4, 61)

	// Should compute rate based on the last 2 samples
	series, err := mRate.flush(65)
	assert.Nil(t, err)
	assert.Len(t, series, 1)
	assert.Len(t, series[0].Points, 1)
	assert.InEpsilon(t, 2./6., series[0].Points[0][1], epsilon)
	assert.EqualValues(t, series[0].Points[0][0], 61)
}

func TestRateSamplingNoSampleForOneFlush(t *testing.T) {
	// Initialize rate
	mRate := Rate{}

	// Add samples
	mRate.addSample(1, 50)
	mRate.addSample(2, 55)

	// First flush: no error
	_, err := mRate.flush(60)
	assert.Nil(t, err)

	// Second flush w/o sample: error
	_, err = mRate.flush(60)
	assert.NotNil(t, err)

	// Third flush w/ sample
	mRate.addSample(4, 60)
	// Should compute rate based on the last 2 samples
	series, err := mRate.flush(60)
	assert.Nil(t, err)
	assert.Len(t, series, 1)
	assert.Len(t, series[0].Points, 1)
	assert.InEpsilon(t, 2./5., series[0].Points[0][1], epsilon)
	assert.EqualValues(t, series[0].Points[0][0], 60)
}

func TestHistogramSampling(t *testing.T) {
	// Initialize histogram
	mHistogram := Histogram{}

	// Empty flush
	_, err := mHistogram.flush(50)
	assert.NotNil(t, err)

	// Add samples
	mHistogram.addSample(1, 50)
	mHistogram.addSample(10, 51)
	mHistogram.addSample(4, 55)
	mHistogram.addSample(5, 55)
	mHistogram.addSample(2, 55)
	mHistogram.addSample(2, 55)

	series, err := mHistogram.flush(60)
	assert.Nil(t, err)
	for _, serie := range series {
		assert.Len(t, serie.Points, 1)
		assert.EqualValues(t, serie.Points[0][0], 60)
	}
	if assert.Len(t, series, 4) {
		assert.InEpsilon(t, 10, series[0].Points[0][1], epsilon)     // max
		assert.Equal(t, ".max", series[0].nameSuffix)                // max
		assert.InEpsilon(t, 2, series[1].Points[0][1], epsilon)      // median
		assert.Equal(t, ".median", series[1].nameSuffix)             // median
		assert.InEpsilon(t, 12./3., series[2].Points[0][1], epsilon) // avg
		assert.Equal(t, ".avg", series[2].nameSuffix)                // avg
		assert.InEpsilon(t, 6, series[3].Points[0][1], epsilon)      // count
		assert.Equal(t, ".count", series[3].nameSuffix)              // count
	}

	_, err = mHistogram.flush(61)
	assert.NotNil(t, err)
}
