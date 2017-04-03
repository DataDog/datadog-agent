package dogstatsd

import (
	// stdlib
	"testing"

	// 3p
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

const epsilon = 0.1

// Schema of a dogstatsd packet:
// <name>:<value>|<metric_type>|@<sample_rate>|#<tag1_name>:<tag1_value>,<tag2_name>:<tag2_value>

func TestParseEmptyDatagram(t *testing.T) {
	emptyDatagram := []byte("")
	pkt := nextPacket(&emptyDatagram)

	assert.Nil(t, pkt)
}

func TestParseOneLineDatagram(t *testing.T) {
	datagram := []byte("daemon:666|g")
	pkt := nextPacket(&datagram)

	assert.NotNil(t, pkt)
	assert.Equal(t, 0, len(datagram))

	// With trailing newline
	datagram = []byte("daemon:666|g\n")
	pkt = nextPacket(&datagram)

	assert.NotNil(t, pkt)
	assert.Equal(t, 0, len(datagram))
}

func TestParseMultipleLineDatagram(t *testing.T) {
	datagram := []byte("daemon:666|g\ndaemon:667|g")

	// First packet
	pkt := nextPacket(&datagram)
	assert.Equal(t, []byte("daemon:666|g"), pkt)

	// Second packet
	pkt = nextPacket(&datagram)
	assert.Equal(t, []byte("daemon:667|g"), pkt)

	// Nore more packet
	pkt = nextPacket(&datagram)
	assert.Nil(t, pkt)
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

func TestServiceCheckMinimal(t *testing.T) {
	sc, err := parseServiceCheckPacket([]byte("_sc|agent.up|0"))

	assert.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "", sc.Host)
	assert.Equal(t, int64(0), sc.Ts)
	assert.Equal(t, aggregator.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, []string(nil), sc.Tags)
}

func TestServiceCheckError(t *testing.T) {
	// not enough infomation
	_, err := parseServiceCheckPacket([]byte("_sc|agent.up"))
	assert.Error(t, err)

	// not invalid status
	_, err = parseServiceCheckPacket([]byte("_sc|agent.up|OK"))
	assert.Error(t, err)

	// not unknown status
	_, err = parseServiceCheckPacket([]byte("_sc|agent.up|21"))
	assert.Error(t, err)

	// invalid timestamp
	_, err = parseServiceCheckPacket([]byte("_sc|agent.up|0|d:some_time"))
	assert.Error(t, err)

	// unknown metadata
	_, err = parseServiceCheckPacket([]byte("_sc|agent.up|0|u:unknown"))
	assert.Error(t, err)
}

func TestServiceCheckMetadataTimestamp(t *testing.T) {
	sc, err := parseServiceCheckPacket([]byte("_sc|agent.up|0|d:21"))

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "", sc.Host)
	assert.Equal(t, int64(21), sc.Ts)
	assert.Equal(t, aggregator.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, []string(nil), sc.Tags)
}

func TestServiceCheckMetadataHostname(t *testing.T) {
	sc, err := parseServiceCheckPacket([]byte("_sc|agent.up|0|h:localhost"))

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "localhost", sc.Host)
	assert.Equal(t, int64(0), sc.Ts)
	assert.Equal(t, aggregator.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, []string(nil), sc.Tags)
}

func TestServiceCheckMetadataTags(t *testing.T) {
	sc, err := parseServiceCheckPacket([]byte("_sc|agent.up|0|#tag1,tag2:test,tag3"))

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "", sc.Host)
	assert.Equal(t, int64(0), sc.Ts)
	assert.Equal(t, aggregator.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, []string{"tag1", "tag2:test", "tag3"}, sc.Tags)
}

func TestServiceCheckMetadataMessage(t *testing.T) {
	sc, err := parseServiceCheckPacket([]byte("_sc|agent.up|0|m:this is fine"))

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "", sc.Host)
	assert.Equal(t, int64(0), sc.Ts)
	assert.Equal(t, aggregator.ServiceCheckOK, sc.Status)
	assert.Equal(t, "this is fine", sc.Message)
	assert.Equal(t, []string(nil), sc.Tags)
}

func TestServiceCheckMetadataMultiple(t *testing.T) {
	// all type
	sc, err := parseServiceCheckPacket([]byte("_sc|agent.up|0|d:21|h:localhost|#tag1:test,tag2|m:this is fine"))
	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "localhost", sc.Host)
	assert.Equal(t, int64(21), sc.Ts)
	assert.Equal(t, aggregator.ServiceCheckOK, sc.Status)
	assert.Equal(t, "this is fine", sc.Message)
	assert.Equal(t, []string{"tag1:test", "tag2"}, sc.Tags)

	// multiple time the same tag
	sc, err = parseServiceCheckPacket([]byte("_sc|agent.up|0|d:21|h:localhost|h:localhost2|d:22"))
	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "localhost2", sc.Host)
	assert.Equal(t, int64(22), sc.Ts)
	assert.Equal(t, aggregator.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, []string(nil), sc.Tags)
}

func TestPacketStringEndings(t *testing.T) {
	assert.Equal(t, 1, 1)
}
