package dogstatsd_tmp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const epsilon = 0.00001

func TestParseGauge(t *testing.T) {
	sample, err := parseMetricSample([]byte("daemon:666|g"))

	assert.NoError(t, err)

	assert.Equal(t, []byte("daemon"), sample.name)
	assert.InEpsilon(t, 666.0, sample.value, epsilon)
	assert.Equal(t, gaugeType, sample.metricType)
	assert.Equal(t, 0, len(sample.tags))
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
}

func TestParseCounter(t *testing.T) {
	sample, err := parseMetricSample([]byte("daemon:21|c"))

	assert.NoError(t, err)

	assert.Equal(t, []byte("daemon"), sample.name)
	assert.Equal(t, 21.0, sample.value)
	assert.Equal(t, countType, sample.metricType)
	assert.Equal(t, 0, len(sample.tags))
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
}

func TestParseCounterWithTags(t *testing.T) {
	sample, err := parseMetricSample([]byte("custom_counter:1|c|#protocol:http,bench"))

	assert.NoError(t, err)

	assert.Equal(t, []byte("custom_counter"), sample.name)
	assert.Equal(t, 1.0, sample.value)
	assert.Equal(t, countType, sample.metricType)
	assert.Equal(t, 2, len(sample.tags))
	assert.Equal(t, []byte("protocol:http"), sample.tags[0])
	assert.Equal(t, []byte("bench"), sample.tags[1])
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
}

func TestParseHistogram(t *testing.T) {
	sample, err := parseMetricSample([]byte("daemon:21|h"))

	assert.NoError(t, err)

	assert.Equal(t, []byte("daemon"), sample.name)
	assert.Equal(t, 21.0, sample.value)
	assert.Equal(t, histogramType, sample.metricType)
	assert.Equal(t, 0, len(sample.tags))
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
}

func TestParseTimer(t *testing.T) {
	sample, err := parseMetricSample([]byte("daemon:21|ms"))

	assert.NoError(t, err)

	assert.Equal(t, []byte("daemon"), sample.name)
	assert.Equal(t, 21.0, sample.value)
	assert.Equal(t, timingType, sample.metricType)
	assert.Equal(t, 0, len(sample.tags))
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
}

func TestParseSet(t *testing.T) {
	sample, err := parseMetricSample([]byte("daemon:abc|s"))

	assert.NoError(t, err)

	assert.Equal(t, []byte("daemon"), sample.name)
	assert.Equal(t, []byte("abc"), sample.setValue)
	assert.Equal(t, setType, sample.metricType)
	assert.Equal(t, 0, len(sample.tags))
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
}

func TestSampleistribution(t *testing.T) {
	sample, err := parseMetricSample([]byte("daemon:3.5|d"))

	assert.NoError(t, err)

	assert.Equal(t, []byte("daemon"), sample.name)
	assert.Equal(t, 3.5, sample.value)
	assert.Equal(t, distributionType, sample.metricType)
	assert.Equal(t, 0, len(sample.tags))
}

func TestParseSetUnicode(t *testing.T) {
	sample, err := parseMetricSample([]byte("daemon:♬†øU†øU¥ºuT0♪|s"))

	assert.NoError(t, err)

	assert.Equal(t, []byte("daemon"), sample.name)
	assert.Equal(t, []byte("♬†øU†øU¥ºuT0♪"), sample.setValue)
	assert.Equal(t, setType, sample.metricType)
	assert.Equal(t, 0, len(sample.tags))
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
}

func TestParseGaugeWithTags(t *testing.T) {
	sample, err := parseMetricSample([]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"))

	assert.NoError(t, err)

	assert.Equal(t, []byte("daemon"), sample.name)
	assert.InEpsilon(t, 666.0, sample.value, epsilon)
	assert.Equal(t, gaugeType, sample.metricType)
	require.Equal(t, 2, len(sample.tags))
	assert.Equal(t, []byte("sometag1:somevalue1"), sample.tags[0])
	assert.Equal(t, []byte("sometag2:somevalue2"), sample.tags[1])
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
}

func TestParseGaugeWithNoTags(t *testing.T) {
	sample, err := parseMetricSample([]byte("daemon:666|g"))
	assert.NoError(t, err)

	assert.Equal(t, []byte("daemon"), sample.name)
	assert.InEpsilon(t, 666.0, sample.value, epsilon)
	assert.Equal(t, gaugeType, sample.metricType)
	assert.Empty(t, sample.tags)
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
}

func TestParseGaugeWithSampleRate(t *testing.T) {
	sample, err := parseMetricSample([]byte("daemon:666|g|@0.21"))

	assert.NoError(t, err)

	assert.Equal(t, []byte("daemon"), sample.name)
	assert.InEpsilon(t, 666.0, sample.value, epsilon)
	assert.Equal(t, gaugeType, sample.metricType)
	assert.Equal(t, 0, len(sample.tags))
	assert.InEpsilon(t, 0.21, sample.sampleRate, epsilon)
}

func TestParseGaugeWithPoundOnly(t *testing.T) {
	sample, err := parseMetricSample([]byte("daemon:666|g|#"))

	assert.NoError(t, err)

	assert.Equal(t, []byte("daemon"), sample.name)
	assert.InEpsilon(t, 666.0, sample.value, epsilon)
	assert.Equal(t, gaugeType, sample.metricType)
	assert.Equal(t, 0, len(sample.tags))
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
}

func TestParseGaugeWithUnicode(t *testing.T) {
	sample, err := parseMetricSample([]byte("♬†øU†øU¥ºuT0♪:666|g|#intitulé:T0µ"))

	assert.NoError(t, err)

	assert.Equal(t, []byte("♬†øU†øU¥ºuT0♪"), sample.name)
	assert.InEpsilon(t, 666.0, sample.value, epsilon)
	assert.Equal(t, gaugeType, sample.metricType)
	require.Equal(t, 1, len(sample.tags))
	assert.Equal(t, []byte("intitulé:T0µ"), sample.tags[0])
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
}

func TestParseMetricError(t *testing.T) {
	// not enough information
	_, err := parseMetricSample([]byte("daemon:666"))
	assert.Error(t, err)

	_, err = parseMetricSample([]byte("daemon:666|"))
	assert.Error(t, err)

	_, err = parseMetricSample([]byte("daemon:|g"))
	assert.Error(t, err)

	_, err = parseMetricSample([]byte(":666|g"))
	assert.Error(t, err)

	_, err = parseMetricSample([]byte("abc666|g"))
	assert.Error(t, err)

	// too many value
	_, err = parseMetricSample([]byte("daemon:666:777|g"))
	assert.Error(t, err)

	// unknown metadata prefix
	_, err = parseMetricSample([]byte("daemon:666|g|m:test"))
	assert.NoError(t, err)

	// invalid value
	_, err = parseMetricSample([]byte("daemon:abc|g"))
	assert.Error(t, err)

	// invalid metric type
	_, err = parseMetricSample([]byte("daemon:666|unknown"))
	assert.Error(t, err)

	// invalid sample rate
	_, err = parseMetricSample([]byte("daemon:666|g|@abc"))
	assert.Error(t, err)
}
