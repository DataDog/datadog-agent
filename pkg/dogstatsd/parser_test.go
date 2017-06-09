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

func TestGaugePacketCounter(t *testing.T) {
	assert.Equal(t, 1, 1)
}

func TestParseGauge(t *testing.T) {
	parsed, err := parseMetricPacket([]byte("daemon:666|g"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, "666", parsed.RawValue)
	assert.Equal(t, aggregator.GaugeType, parsed.Mtype)
	assert.Equal(t, 0, len(*(parsed.Tags)))
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestParseCounter(t *testing.T) {
	parsed, err := parseMetricPacket([]byte("daemon:21|c"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, 21.0, parsed.Value)
	assert.Equal(t, aggregator.CounterType, parsed.Mtype)
	assert.Equal(t, 0, len(*(parsed.Tags)))
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestParseHistogram(t *testing.T) {
	parsed, err := parseMetricPacket([]byte("daemon:21|h"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, 21.0, parsed.Value)
	assert.Equal(t, aggregator.HistogramType, parsed.Mtype)
	assert.Equal(t, 0, len(*(parsed.Tags)))
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestParseTimer(t *testing.T) {
	parsed, err := parseMetricPacket([]byte("daemon:21|ms"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, 21.0, parsed.Value)
	assert.Equal(t, aggregator.HistogramType, parsed.Mtype)
	assert.Equal(t, 0, len(*(parsed.Tags)))
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestParseSet(t *testing.T) {
	parsed, err := parseMetricPacket([]byte("daemon:abc|s"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, "abc", parsed.RawValue)
	assert.Equal(t, aggregator.SetType, parsed.Mtype)
	assert.Equal(t, 0, len(*(parsed.Tags)))
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestParseDistribution(t *testing.T) {
	parsed, err := parseMetricPacket([]byte("daemon:3.5|d"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, 3.5, parsed.Value)
	assert.Equal(t, aggregator.DistributionType, parsed.Mtype)
	assert.Equal(t, 0, len(*(parsed.Tags)))
}

func TestParseSetUnicode(t *testing.T) {
	parsed, err := parseMetricPacket([]byte("daemon:♬†øU†øU¥ºuT0♪|s"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, "♬†øU†øU¥ºuT0♪", parsed.RawValue)
	assert.Equal(t, aggregator.SetType, parsed.Mtype)
	assert.Equal(t, 0, len(*(parsed.Tags)))
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestParseGaugeWithTags(t *testing.T) {
	parsed, err := parseMetricPacket([]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, aggregator.GaugeType, parsed.Mtype)
	require.Equal(t, 2, len(*(parsed.Tags)))
	assert.Equal(t, "sometag1:somevalue1", (*parsed.Tags)[0])
	assert.Equal(t, "sometag2:somevalue2", (*parsed.Tags)[1])
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestParseGaugeWithSampleRate(t *testing.T) {
	parsed, err := parseMetricPacket([]byte("daemon:666|g|@0.21"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, aggregator.GaugeType, parsed.Mtype)
	assert.Equal(t, 0, len(*(parsed.Tags)))
	assert.InEpsilon(t, 0.21, parsed.SampleRate, epsilon)
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
	require.Equal(t, 1, len(*(parsed.Tags)))
	assert.Equal(t, "intitulé:T0µ", (*parsed.Tags)[0])
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestParseMetricError(t *testing.T) {
	// not enough infomation
	_, err := parseMetricPacket([]byte("daemon:666"))
	assert.Error(t, err)

	_, err = parseMetricPacket([]byte("daemon:666|"))
	assert.Error(t, err)

	_, err = parseMetricPacket([]byte("daemon:|g"))
	assert.Error(t, err)

	_, err = parseMetricPacket([]byte(":666|g"))
	assert.Error(t, err)

	// too many value
	_, err = parseMetricPacket([]byte("daemon:666:777|g"))
	assert.Error(t, err)

	// unknown metadata prefix
	_, err = parseMetricPacket([]byte("daemon:666|g|m:test"))
	assert.NoError(t, err)

	// invalid value
	_, err = parseMetricPacket([]byte("daemon:abc|g"))
	assert.Error(t, err)

	// invalid metric type
	_, err = parseMetricPacket([]byte("daemon:666|unknown"))
	assert.Error(t, err)

	// invalid sample rate
	_, err = parseMetricPacket([]byte("daemon:666|g|@abc"))
	assert.Error(t, err)
}

func TestParseMonokeyBatching(t *testing.T) {
	// parsed, err := parseMetricPacket([]byte("test_gauge:1.5|g|#tag1:one,tag2:two:2.3|g|#tag3:three:3|g"))

	// TODO: implement test
}

func TestEnsureUTF8(t *testing.T) {
	assert.Equal(t, 1, 1)
}

func TestMagicTags(t *testing.T) { // ie host:test-b
	assert.Equal(t, 1, 1)
}

func TestScientificNotation(t *testing.T) {
	assert.Equal(t, 1, 1)
}

func TestPacketStringEndings(t *testing.T) {
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

	_, err = parseServiceCheckPacket([]byte("_sc|agent.up|"))
	assert.Error(t, err)

	// not invalid status
	_, err = parseServiceCheckPacket([]byte("_sc|agent.up|OK"))
	assert.Error(t, err)

	// not unknown status
	_, err = parseServiceCheckPacket([]byte("_sc|agent.up|21"))
	assert.Error(t, err)

	// invalid timestamp
	_, err = parseServiceCheckPacket([]byte("_sc|agent.up|0|d:some_time"))
	assert.NoError(t, err)

	// unknown metadata
	_, err = parseServiceCheckPacket([]byte("_sc|agent.up|0|u:unknown"))
	assert.NoError(t, err)
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

func TestEventMinimal(t *testing.T) {
	e, err := parseEventPacket([]byte("_e{10,9}:test title|test text"))

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, aggregator.EventPriorityNormal, e.Priority)
	assert.Equal(t, "", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, aggregator.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}

func TestEventMultilinesText(t *testing.T) {
	e, err := parseEventPacket([]byte("_e{10,24}:test title|test\\line1\\nline2\\nline3"))

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test\\line1\nline2\nline3", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, aggregator.EventPriorityNormal, e.Priority)
	assert.Equal(t, "", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, aggregator.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}

func TestEventPipeInTitle(t *testing.T) {
	e, err := parseEventPacket([]byte("_e{10,24}:test|title|test\\line1\\nline2\\nline3"))

	require.Nil(t, err)
	assert.Equal(t, "test|title", e.Title)
	assert.Equal(t, "test\\line1\nline2\nline3", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, aggregator.EventPriorityNormal, e.Priority)
	assert.Equal(t, "", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, aggregator.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}

func TestEventError(t *testing.T) {
	// missing length header
	_, err := parseEventPacket([]byte("_e:title|text"))
	assert.Error(t, err)

	// greater length than packet
	_, err = parseEventPacket([]byte("_e{10,10}:title|text"))
	assert.Error(t, err)

	// zero length
	_, err = parseEventPacket([]byte("_e{0,0}:a|a"))
	assert.Error(t, err)

	// missing title or text length
	_, err = parseEventPacket([]byte("_e{5555:title|text"))
	assert.Error(t, err)

	// missing wrong len format
	_, err = parseEventPacket([]byte("_e{a,1}:title|text"))
	assert.Error(t, err)

	_, err = parseEventPacket([]byte("_e{1,a}:title|text"))
	assert.Error(t, err)

	// missing title or text length
	_, err = parseEventPacket([]byte("_e{5,}:title|text"))
	assert.Error(t, err)

	_, err = parseEventPacket([]byte("_e{,4}:title|text"))
	assert.Error(t, err)

	_, err = parseEventPacket([]byte("_e{}:title|text"))
	assert.Error(t, err)

	_, err = parseEventPacket([]byte("_e{,}:title|text"))
	assert.Error(t, err)

	// not enough infomation
	_, err = parseEventPacket([]byte("_e|text"))
	assert.Error(t, err)

	_, err = parseEventPacket([]byte("_e:|text"))
	assert.Error(t, err)

	// invalid timestamp
	_, err = parseEventPacket([]byte("_e{5,4}:title|text|d:abc"))
	assert.NoError(t, err)

	// invalid priority
	_, err = parseEventPacket([]byte("_e{5,4}:title|text|p:urgent"))
	assert.NoError(t, err)

	// invalid priority
	_, err = parseEventPacket([]byte("_e{5,4}:title|text|p:urgent"))
	assert.NoError(t, err)

	// invalid alert type
	_, err = parseEventPacket([]byte("_e{5,4}:title|text|t:test"))
	assert.NoError(t, err)

	// unknown metadata
	_, err = parseEventPacket([]byte("_e{5,4}:title|text|x:1234"))
	assert.NoError(t, err)
}

func TestEventMetadataTimestamp(t *testing.T) {
	e, err := parseEventPacket([]byte("_e{10,9}:test title|test text|d:21"))

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(21), e.Ts)
	assert.Equal(t, aggregator.EventPriorityNormal, e.Priority)
	assert.Equal(t, "", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, aggregator.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}

func TestEventMetadataPriority(t *testing.T) {
	e, err := parseEventPacket([]byte("_e{10,9}:test title|test text|p:low"))

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, aggregator.EventPriorityLow, e.Priority)
	assert.Equal(t, "", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, aggregator.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}

func TestEventMetadataHostname(t *testing.T) {
	e, err := parseEventPacket([]byte("_e{10,9}:test title|test text|h:localhost"))

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, aggregator.EventPriorityNormal, e.Priority)
	assert.Equal(t, "localhost", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, aggregator.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}

func TestEventMetadataAlertType(t *testing.T) {
	e, err := parseEventPacket([]byte("_e{10,9}:test title|test text|t:warning"))

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, aggregator.EventPriorityNormal, e.Priority)
	assert.Equal(t, "", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, aggregator.EventAlertTypeWarning, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}

func TestEventMetadataAggregatioKey(t *testing.T) {
	e, err := parseEventPacket([]byte("_e{10,9}:test title|test text|k:some aggregation key"))

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, aggregator.EventPriorityNormal, e.Priority)
	assert.Equal(t, "", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, aggregator.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "some aggregation key", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}

func TestEventMetadataSourceType(t *testing.T) {
	e, err := parseEventPacket([]byte("_e{10,9}:test title|test text|s:this is the source"))

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, aggregator.EventPriorityNormal, e.Priority)
	assert.Equal(t, "", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, aggregator.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "this is the source", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}

func TestEventMetadataTags(t *testing.T) {
	e, err := parseEventPacket([]byte("_e{10,9}:test title|test text|#tag1,tag2:test"))

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, aggregator.EventPriorityNormal, e.Priority)
	assert.Equal(t, "", e.Host)
	assert.Equal(t, []string{"tag1", "tag2:test"}, e.Tags)
	assert.Equal(t, aggregator.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}

func TestEventMetadataMultiple(t *testing.T) {
	e, err := parseEventPacket([]byte("_e{10,9}:test title|test text|t:warning|d:12345|p:low|h:some.host|k:aggKey|s:source test|#tag1,tag2:test"))

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(12345), e.Ts)
	assert.Equal(t, aggregator.EventPriorityLow, e.Priority)
	assert.Equal(t, "some.host", e.Host)
	assert.Equal(t, []string{"tag1", "tag2:test"}, e.Tags)
	assert.Equal(t, aggregator.EventAlertTypeWarning, e.AlertType)
	assert.Equal(t, "aggKey", e.AggregationKey)
	assert.Equal(t, "source test", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}
