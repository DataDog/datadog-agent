package dogstatsd

import (
	// stdlib
	"testing"

	// 3p
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

const epsilon = 0.1

// Schema of a dogstatsd packet:
// <name>:<value>|<metric_type>|@<sample_rate>|#<tag1_name>:<tag1_value>,<tag2_name>:<tag2_value>

func TestParseEmptyDatagram(t *testing.T) {
	emptyDatagram := []byte("")
	sample, err := nextMetric(&emptyDatagram)

	assert.NoError(t, err)
	assert.Nil(t, sample)
}

func TestParseOneLineDatagram(t *testing.T) {
	datagram := []byte("daemon:666|g")
	sample, err := nextMetric(&datagram)

	assert.NoError(t, err)
	assert.NotNil(t, sample)
	assert.Equal(t, 0, len(datagram))

	// With trailing newline
	datagram = []byte("daemon:666|g\n")
	sample, err = nextMetric(&datagram)

	assert.NoError(t, err)
	assert.NotNil(t, sample)
	assert.Equal(t, 0, len(datagram))
}

func TestParseMultipleLineDatagram(t *testing.T) {
	datagram := []byte("daemon:666|g\ndaemon:666|g")

	// First sample
	sample, err := nextMetric(&datagram)
	assert.NoError(t, err)
	assert.NotNil(t, sample)

	// Second sample
	sample, err = nextMetric(&datagram)
	assert.NoError(t, err)
	assert.NotNil(t, sample)

	// Nore more samples
	sample, err = nextMetric(&datagram)
	assert.NoError(t, err)
	assert.Nil(t, sample)
	assert.Equal(t, 0, len(datagram))
}

func TestSubmitPacketToAggregator(t *testing.T) {
	assert.Equal(t, 1, 1)
}

func TestGaugePacketCounter(t *testing.T) {
	assert.Equal(t, 1, 1)
}

func TestParseGauge(t *testing.T) {
	parsed, err := parseMetricPacket([]byte("daemon:666|g"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, aggregator.GaugeType, parsed.Mtype)
	assert.Equal(t, 0, len(*(parsed.Tags)))
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestParseGaugeWithTags(t *testing.T) {
	parsed, err := parseMetricPacket([]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, aggregator.GaugeType, parsed.Mtype)
	if assert.Equal(t, 2, len(*(parsed.Tags))) {
		assert.Equal(t, "sometag1:somevalue1", (*parsed.Tags)[0])
		assert.Equal(t, "sometag2:somevalue2", (*parsed.Tags)[1])
	}
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestParseGaugeWithPoundOnly(t *testing.T) {
	parsed, err := parseMetricPacket([]byte("daemon:666|g|#"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, aggregator.GaugeType, parsed.Mtype)
	assert.Equal(t, 0, len(*(parsed.Tags)))
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestParseGaugeWithUnicode(t *testing.T) {
	parsed, err := parseMetricPacket([]byte("♬†øU†øU¥ºuT0♪:666|g|#intitulé:T0µ"))

	assert.NoError(t, err)

	assert.Equal(t, "♬†øU†øU¥ºuT0♪", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, aggregator.GaugeType, parsed.Mtype)
	if assert.Equal(t, 1, len(*(parsed.Tags))) {
		assert.Equal(t, "intitulé:T0µ", (*parsed.Tags)[0])
	}
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestParseMonokeyBatching(t *testing.T) {
	// parsed, err := parseMetricPacket([]byte("test_gauge:1.5|g|#tag1:one,tag2:two:2.3|g|#tag3:three:3|g"))

	// TODO: implement test
}

func TestEnsureUTF8(t *testing.T) {
	assert.Equal(t, 1, 1)
}

func TestTags(t *testing.T) {
	assert.Equal(t, 1, 1)
}

func TestMagicTags(t *testing.T) { // ie host:test-b
	assert.Equal(t, 1, 1)
}

func TestScientificNotation(t *testing.T) {
	assert.Equal(t, 1, 1)
}

func TestEventTags(t *testing.T) {
	assert.Equal(t, 1, 1)
}

func TestEventTitle(t *testing.T) {
	assert.Equal(t, 1, 1)
}

func TestEventText(t *testing.T) {
	assert.Equal(t, 1, 1)
}

func TestEventTextUTF8(t *testing.T) {
	assert.Equal(t, 1, 1)
}

func TestServiceCheckMessage(t *testing.T) {
	assert.Equal(t, 1, 1)
}

func TestServiceCheckTags(t *testing.T) {
	assert.Equal(t, 1, 1)
}

func TestPacketStringEndings(t *testing.T) {
	assert.Equal(t, 1, 1)
}
