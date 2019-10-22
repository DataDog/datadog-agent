package dogstatsd_tmp

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseMetricMessage(message []byte, namespace string, namespaceBlacklist []string, defaultHostname string) (*metrics.MetricSample, error) {
	sample, err := parseMetricSample(message)
	if err != nil {
		return nil, err
	}
	return convertMetricSample(sample, namespace, namespaceBlacklist, defaultHostname), nil
}

func TestConvertParseGauge(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:666|g"), "", nil, "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, metrics.GaugeType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertParseCounter(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:21|c"), "", nil, "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, 21.0, parsed.Value)
	assert.Equal(t, metrics.CounterType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertParseCounterWithTags(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("custom_counter:1|c|#protocol:http,bench"), "", nil, "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "custom_counter", parsed.Name)
	assert.Equal(t, 1.0, parsed.Value)
	assert.Equal(t, metrics.CounterType, parsed.Mtype)
	assert.Equal(t, 2, len(parsed.Tags))
	assert.Equal(t, "protocol:http", parsed.Tags[0])
	assert.Equal(t, "bench", parsed.Tags[1])
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertParseHistogram(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:21|h"), "", nil, "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, 21.0, parsed.Value)
	assert.Equal(t, metrics.HistogramType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertParseTimer(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:21|ms"), "", nil, "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, 21.0, parsed.Value)
	assert.Equal(t, metrics.HistogramType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
	assert.Equal(t, "default-hostname", parsed.Host)
}

func TestConvertParseSet(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:abc|s"), "", nil, "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, "abc", parsed.RawValue)
	assert.Equal(t, metrics.SetType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertParseDistribution(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:3.5|d"), "", nil, "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, 3.5, parsed.Value)
	assert.Equal(t, metrics.DistributionType, parsed.Mtype)
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.Equal(t, 0, len(parsed.Tags))
}

func TestConvertParseSetUnicode(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:♬†øU†øU¥ºuT0♪|s"), "", nil, "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, "♬†øU†øU¥ºuT0♪", parsed.RawValue)
	assert.Equal(t, metrics.SetType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertParseGaugeWithTags(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"), "", nil, "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, metrics.GaugeType, parsed.Mtype)
	require.Equal(t, 2, len(parsed.Tags))
	assert.Equal(t, "sometag1:somevalue1", parsed.Tags[0])
	assert.Equal(t, "sometag2:somevalue2", parsed.Tags[1])
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertParseGaugeWithHostTag(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:666|g|#sometag1:somevalue1,host:my-hostname,sometag2:somevalue2"), "", nil, "default-hostname")
	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, metrics.GaugeType, parsed.Mtype)
	require.Equal(t, 2, len(parsed.Tags))
	assert.Equal(t, "sometag1:somevalue1", parsed.Tags[0])
	assert.Equal(t, "sometag2:somevalue2", parsed.Tags[1])
	assert.Equal(t, "my-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertParseGaugeWithEmptyHostTag(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:666|g|#sometag1:somevalue1,host:,sometag2:somevalue2"), "", nil, "default-hostname")
	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, metrics.GaugeType, parsed.Mtype)
	require.Equal(t, 2, len(parsed.Tags))
	assert.Equal(t, "sometag1:somevalue1", parsed.Tags[0])
	assert.Equal(t, "sometag2:somevalue2", parsed.Tags[1])
	assert.Equal(t, "", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertParseGaugeWithNoTags(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:666|g"), "", nil, "default-hostname")
	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, metrics.GaugeType, parsed.Mtype)
	assert.Empty(t, parsed.Tags)
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertParseGaugeWithSampleRate(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:666|g|@0.21"), "", nil, "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, metrics.GaugeType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 0.21, parsed.SampleRate, epsilon)
}

func TestConvertParseGaugeWithPoundOnly(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:666|g|#"), "", nil, "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, metrics.GaugeType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertParseGaugeWithUnicode(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("♬†øU†øU¥ºuT0♪:666|g|#intitulé:T0µ"), "", nil, "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "♬†øU†øU¥ºuT0♪", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, metrics.GaugeType, parsed.Mtype)
	require.Equal(t, 1, len(parsed.Tags))
	assert.Equal(t, "intitulé:T0µ", parsed.Tags[0])
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertParseMetricError(t *testing.T) {
	// not enough information
	_, err := parseMetricMessage([]byte("daemon:666"), "", nil, "default-hostname")
	assert.Error(t, err)

	_, err = parseMetricMessage([]byte("daemon:666|"), "", nil, "default-hostname")
	assert.Error(t, err)

	_, err = parseMetricMessage([]byte("daemon:|g"), "", nil, "default-hostname")
	assert.Error(t, err)

	_, err = parseMetricMessage([]byte(":666|g"), "", nil, "default-hostname")
	assert.Error(t, err)

	// too many value
	_, err = parseMetricMessage([]byte("daemon:666:777|g"), "", nil, "default-hostname")
	assert.Error(t, err)

	// unknown metadata prefix
	_, err = parseMetricMessage([]byte("daemon:666|g|m:test"), "", nil, "default-hostname")
	assert.NoError(t, err)

	// invalid value
	_, err = parseMetricMessage([]byte("daemon:abc|g"), "", nil, "default-hostname")
	assert.Error(t, err)

	// invalid metric type
	_, err = parseMetricMessage([]byte("daemon:666|unknown"), "", nil, "default-hostname")
	assert.Error(t, err)

	// invalid sample rate
	_, err = parseMetricMessage([]byte("daemon:666|g|@abc"), "", nil, "default-hostname")
	assert.Error(t, err)
}
