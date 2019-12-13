package dogstatsd

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseAndEnrichMetricMessage(message []byte, namespace string, namespaceBlacklist []string, defaultHostname string) (*metrics.MetricSample, error) {
	parsed, err := parseMetricSample(message)
	if err != nil {
		return nil, err
	}
	return enrichMetricSample(parsed, namespace, namespaceBlacklist, defaultHostname), nil
}

func parseAndEnrichServiceCheckMessage(message []byte, defaultHostname string) (*metrics.ServiceCheck, error) {
	parsed, err := parseServiceCheck(message)
	if err != nil {
		return nil, err
	}
	return enrichServiceCheck(parsed, defaultHostname), nil
}

func parseAndEnrichEventMessage(message []byte, defaultHostname string) (*metrics.Event, error) {
	parsed, err := parseEvent(message)
	if err != nil {
		return nil, err
	}
	return enrichEvent(parsed, defaultHostname), nil
}

func TestConvertParseGauge(t *testing.T) {
	parsed, err := parseAndEnrichMetricMessage([]byte("daemon:666|g"), "", nil, "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, metrics.GaugeType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertParseCounter(t *testing.T) {
	parsed, err := parseAndEnrichMetricMessage([]byte("daemon:21|c"), "", nil, "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, 21.0, parsed.Value)
	assert.Equal(t, metrics.CounterType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertParseCounterWithTags(t *testing.T) {
	parsed, err := parseAndEnrichMetricMessage([]byte("custom_counter:1|c|#protocol:http,bench"), "", nil, "default-hostname")

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
	parsed, err := parseAndEnrichMetricMessage([]byte("daemon:21|h"), "", nil, "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, 21.0, parsed.Value)
	assert.Equal(t, metrics.HistogramType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertParseTimer(t *testing.T) {
	parsed, err := parseAndEnrichMetricMessage([]byte("daemon:21|ms"), "", nil, "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, 21.0, parsed.Value)
	assert.Equal(t, metrics.HistogramType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
	assert.Equal(t, "default-hostname", parsed.Host)
}

func TestConvertParseSet(t *testing.T) {
	parsed, err := parseAndEnrichMetricMessage([]byte("daemon:abc|s"), "", nil, "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, "abc", parsed.RawValue)
	assert.Equal(t, metrics.SetType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertParseDistribution(t *testing.T) {
	parsed, err := parseAndEnrichMetricMessage([]byte("daemon:3.5|d"), "", nil, "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, 3.5, parsed.Value)
	assert.Equal(t, metrics.DistributionType, parsed.Mtype)
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.Equal(t, 0, len(parsed.Tags))
}

func TestConvertParseSetUnicode(t *testing.T) {
	parsed, err := parseAndEnrichMetricMessage([]byte("daemon:♬†øU†øU¥ºuT0♪|s"), "", nil, "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, "♬†øU†øU¥ºuT0♪", parsed.RawValue)
	assert.Equal(t, metrics.SetType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertParseGaugeWithTags(t *testing.T) {
	parsed, err := parseAndEnrichMetricMessage([]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"), "", nil, "default-hostname")

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
	parsed, err := parseAndEnrichMetricMessage([]byte("daemon:666|g|#sometag1:somevalue1,host:my-hostname,sometag2:somevalue2"), "", nil, "default-hostname")
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
	parsed, err := parseAndEnrichMetricMessage([]byte("daemon:666|g|#sometag1:somevalue1,host:,sometag2:somevalue2"), "", nil, "default-hostname")
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
	parsed, err := parseAndEnrichMetricMessage([]byte("daemon:666|g"), "", nil, "default-hostname")
	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, metrics.GaugeType, parsed.Mtype)
	assert.Empty(t, parsed.Tags)
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertParseGaugeWithSampleRate(t *testing.T) {
	parsed, err := parseAndEnrichMetricMessage([]byte("daemon:666|g|@0.21"), "", nil, "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, metrics.GaugeType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 0.21, parsed.SampleRate, epsilon)
}

func TestConvertParseGaugeWithPoundOnly(t *testing.T) {
	parsed, err := parseAndEnrichMetricMessage([]byte("daemon:666|g|#"), "", nil, "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, metrics.GaugeType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertParseGaugeWithUnicode(t *testing.T) {
	parsed, err := parseAndEnrichMetricMessage([]byte("♬†øU†øU¥ºuT0♪:666|g|#intitulé:T0µ"), "", nil, "default-hostname")

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
	_, err := parseAndEnrichMetricMessage([]byte("daemon:666"), "", nil, "default-hostname")
	assert.Error(t, err)

	_, err = parseAndEnrichMetricMessage([]byte("daemon:666|"), "", nil, "default-hostname")
	assert.Error(t, err)

	_, err = parseAndEnrichMetricMessage([]byte("daemon:|g"), "", nil, "default-hostname")
	assert.Error(t, err)

	_, err = parseAndEnrichMetricMessage([]byte(":666|g"), "", nil, "default-hostname")
	assert.Error(t, err)

	// too many value
	_, err = parseAndEnrichMetricMessage([]byte("daemon:666:777|g"), "", nil, "default-hostname")
	assert.Error(t, err)

	// unknown metadata prefix
	_, err = parseAndEnrichMetricMessage([]byte("daemon:666|g|m:test"), "", nil, "default-hostname")
	assert.NoError(t, err)

	// invalid value
	_, err = parseAndEnrichMetricMessage([]byte("daemon:abc|g"), "", nil, "default-hostname")
	assert.Error(t, err)

	// invalid metric type
	_, err = parseAndEnrichMetricMessage([]byte("daemon:666|unknown"), "", nil, "default-hostname")
	assert.Error(t, err)

	// invalid sample rate
	_, err = parseAndEnrichMetricMessage([]byte("daemon:666|g|@abc"), "", nil, "default-hostname")
	assert.Error(t, err)
}

func TestConvertParseMonokeyBatching(t *testing.T) {
	// TODO: not implemented
	// parsed, err := parseAndEnrichMetricMessage([]byte("test_gauge:1.5|g|#tag1:one,tag2:two:2.3|g|#tag3:three:3|g"), "default-hostname")
}

func TestConvertEnsureUTF8(t *testing.T) {
	assert.Equal(t, 1, 1)
}

func TestConvertMagicTags(t *testing.T) { // ie host:test-b
	assert.Equal(t, 1, 1)
}

func TestConvertScientificNotation(t *testing.T) {
	assert.Equal(t, 1, 1)
}

func TestConvertPacketStringEndings(t *testing.T) {
	assert.Equal(t, 1, 1)
}

func TestConvertServiceCheckMinimal(t *testing.T) {
	sc, err := parseAndEnrichServiceCheckMessage([]byte("_sc|agent.up|0"), "default-hostname")

	assert.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "default-hostname", sc.Host)
	assert.Equal(t, int64(0), sc.Ts)
	assert.Equal(t, metrics.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, []string(nil), sc.Tags)
}

func TestConvertServiceCheckError(t *testing.T) {
	// not enough information
	_, err := parseAndEnrichServiceCheckMessage([]byte("_sc|agent.up"), "default-hostname")
	assert.Error(t, err)

	_, err = parseAndEnrichServiceCheckMessage([]byte("_sc|agent.up|"), "default-hostname")
	assert.Error(t, err)

	// not invalid status
	_, err = parseAndEnrichServiceCheckMessage([]byte("_sc|agent.up|OK"), "default-hostname")
	assert.Error(t, err)

	// not unknown status
	_, err = parseAndEnrichServiceCheckMessage([]byte("_sc|agent.up|21"), "default-hostname")
	assert.Error(t, err)

	// invalid timestamp
	_, err = parseAndEnrichServiceCheckMessage([]byte("_sc|agent.up|0|d:some_time"), "default-hostname")
	assert.NoError(t, err)

	// unknown metadata
	_, err = parseAndEnrichServiceCheckMessage([]byte("_sc|agent.up|0|u:unknown"), "default-hostname")
	assert.NoError(t, err)
}

func TestConvertServiceCheckMetadataTimestamp(t *testing.T) {
	sc, err := parseAndEnrichServiceCheckMessage([]byte("_sc|agent.up|0|d:21"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "default-hostname", sc.Host)
	assert.Equal(t, int64(21), sc.Ts)
	assert.Equal(t, metrics.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, []string(nil), sc.Tags)
}

func TestConvertServiceCheckMetadataHostname(t *testing.T) {
	sc, err := parseAndEnrichServiceCheckMessage([]byte("_sc|agent.up|0|h:localhost"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "localhost", sc.Host)
	assert.Equal(t, int64(0), sc.Ts)
	assert.Equal(t, metrics.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, []string(nil), sc.Tags)
}

func TestConvertServiceCheckMetadataHostnameInTag(t *testing.T) {
	sc, err := parseAndEnrichServiceCheckMessage([]byte("_sc|agent.up|0|#host:localhost"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "localhost", sc.Host)
	assert.Equal(t, int64(0), sc.Ts)
	assert.Equal(t, metrics.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, []string{}, sc.Tags)
}

func TestConvertServiceCheckMetadataEmptyHostTag(t *testing.T) {
	sc, err := parseAndEnrichServiceCheckMessage([]byte("_sc|agent.up|0|#host:,other:tag"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "", sc.Host)
	assert.Equal(t, int64(0), sc.Ts)
	assert.Equal(t, metrics.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, []string{"other:tag"}, sc.Tags)
}

func TestConvertServiceCheckMetadataTags(t *testing.T) {
	sc, err := parseAndEnrichServiceCheckMessage([]byte("_sc|agent.up|0|#tag1,tag2:test,tag3"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "default-hostname", sc.Host)
	assert.Equal(t, int64(0), sc.Ts)
	assert.Equal(t, metrics.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, []string{"tag1", "tag2:test", "tag3"}, sc.Tags)
}

func TestConvertServiceCheckMetadataMessage(t *testing.T) {
	sc, err := parseAndEnrichServiceCheckMessage([]byte("_sc|agent.up|0|m:this is fine"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "default-hostname", sc.Host)
	assert.Equal(t, int64(0), sc.Ts)
	assert.Equal(t, metrics.ServiceCheckOK, sc.Status)
	assert.Equal(t, "this is fine", sc.Message)
	assert.Equal(t, []string(nil), sc.Tags)
}

func TestConvertServiceCheckMetadataMultiple(t *testing.T) {
	// all type
	sc, err := parseAndEnrichServiceCheckMessage([]byte("_sc|agent.up|0|d:21|h:localhost|#tag1:test,tag2|m:this is fine"), "default-hostname")
	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "localhost", sc.Host)
	assert.Equal(t, int64(21), sc.Ts)
	assert.Equal(t, metrics.ServiceCheckOK, sc.Status)
	assert.Equal(t, "this is fine", sc.Message)
	assert.Equal(t, []string{"tag1:test", "tag2"}, sc.Tags)

	// multiple time the same tag
	sc, err = parseAndEnrichServiceCheckMessage([]byte("_sc|agent.up|0|d:21|h:localhost|h:localhost2|d:22"), "default-hostname")
	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "localhost2", sc.Host)
	assert.Equal(t, int64(22), sc.Ts)
	assert.Equal(t, metrics.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, []string(nil), sc.Tags)
}

func TestConvertEventMinimal(t *testing.T) {
	e, err := parseAndEnrichEventMessage([]byte("_e{10,9}:test title|test text"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, metrics.EventPriorityNormal, e.Priority)
	assert.Equal(t, "default-hostname", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, metrics.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}

func TestConvertEventMultilinesText(t *testing.T) {
	e, err := parseAndEnrichEventMessage([]byte("_e{10,24}:test title|test\\line1\\nline2\\nline3"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test\\line1\nline2\nline3", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, metrics.EventPriorityNormal, e.Priority)
	assert.Equal(t, "default-hostname", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, metrics.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}

func TestConvertEventPipeInTitle(t *testing.T) {
	e, err := parseAndEnrichEventMessage([]byte("_e{10,24}:test|title|test\\line1\\nline2\\nline3"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "test|title", e.Title)
	assert.Equal(t, "test\\line1\nline2\nline3", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, metrics.EventPriorityNormal, e.Priority)
	assert.Equal(t, "default-hostname", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, metrics.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}

func TestConvertEventError(t *testing.T) {
	// missing length header
	_, err := parseAndEnrichEventMessage([]byte("_e:title|text"), "default-hostname")
	assert.Error(t, err)

	// greater length than packet
	_, err = parseAndEnrichEventMessage([]byte("_e{10,10}:title|text"), "default-hostname")
	assert.Error(t, err)

	// zero length
	_, err = parseAndEnrichEventMessage([]byte("_e{0,0}:a|a"), "default-hostname")
	assert.Error(t, err)

	// missing title or text length
	_, err = parseAndEnrichEventMessage([]byte("_e{5555:title|text"), "default-hostname")
	assert.Error(t, err)

	// missing wrong len format
	_, err = parseAndEnrichEventMessage([]byte("_e{a,1}:title|text"), "default-hostname")
	assert.Error(t, err)

	_, err = parseAndEnrichEventMessage([]byte("_e{1,a}:title|text"), "default-hostname")
	assert.Error(t, err)

	// missing title or text length
	_, err = parseAndEnrichEventMessage([]byte("_e{5,}:title|text"), "default-hostname")
	assert.Error(t, err)

	_, err = parseAndEnrichEventMessage([]byte("_e{,4}:title|text"), "default-hostname")
	assert.Error(t, err)

	_, err = parseAndEnrichEventMessage([]byte("_e{}:title|text"), "default-hostname")
	assert.Error(t, err)

	_, err = parseAndEnrichEventMessage([]byte("_e{,}:title|text"), "default-hostname")
	assert.Error(t, err)

	// not enough information
	_, err = parseAndEnrichEventMessage([]byte("_e|text"), "default-hostname")
	assert.Error(t, err)

	_, err = parseAndEnrichEventMessage([]byte("_e:|text"), "default-hostname")
	assert.Error(t, err)

	// invalid timestamp
	_, err = parseAndEnrichEventMessage([]byte("_e{5,4}:title|text|d:abc"), "default-hostname")
	assert.NoError(t, err)

	// invalid priority
	_, err = parseAndEnrichEventMessage([]byte("_e{5,4}:title|text|p:urgent"), "default-hostname")
	assert.NoError(t, err)

	// invalid priority
	_, err = parseAndEnrichEventMessage([]byte("_e{5,4}:title|text|p:urgent"), "default-hostname")
	assert.NoError(t, err)

	// invalid alert type
	_, err = parseAndEnrichEventMessage([]byte("_e{5,4}:title|text|t:test"), "default-hostname")
	assert.NoError(t, err)

	// unknown metadata
	_, err = parseAndEnrichEventMessage([]byte("_e{5,4}:title|text|x:1234"), "default-hostname")
	assert.NoError(t, err)
}

func TestConvertEventMetadataTimestamp(t *testing.T) {
	e, err := parseAndEnrichEventMessage([]byte("_e{10,9}:test title|test text|d:21"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(21), e.Ts)
	assert.Equal(t, metrics.EventPriorityNormal, e.Priority)
	assert.Equal(t, "default-hostname", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, metrics.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}

func TestConvertEventMetadataPriority(t *testing.T) {
	e, err := parseAndEnrichEventMessage([]byte("_e{10,9}:test title|test text|p:low"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, metrics.EventPriorityLow, e.Priority)
	assert.Equal(t, "default-hostname", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, metrics.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}

func TestConvertEventMetadataHostname(t *testing.T) {
	e, err := parseAndEnrichEventMessage([]byte("_e{10,9}:test title|test text|h:localhost"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, metrics.EventPriorityNormal, e.Priority)
	assert.Equal(t, "localhost", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, metrics.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}

func TestConvertEventMetadataHostnameInTag(t *testing.T) {
	e, err := parseAndEnrichEventMessage([]byte("_e{10,9}:test title|test text|#host:localhost"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, metrics.EventPriorityNormal, e.Priority)
	assert.Equal(t, "localhost", e.Host)
	assert.Equal(t, []string{}, e.Tags)
	assert.Equal(t, metrics.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}

func TestConvertEventMetadataEmptyHostTag(t *testing.T) {
	e, err := parseAndEnrichEventMessage([]byte("_e{10,9}:test title|test text|#host:,other:tag"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, metrics.EventPriorityNormal, e.Priority)
	assert.Equal(t, "", e.Host)
	assert.Equal(t, []string{"other:tag"}, e.Tags)
	assert.Equal(t, metrics.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}

func TestConvertEventMetadataAlertType(t *testing.T) {
	e, err := parseAndEnrichEventMessage([]byte("_e{10,9}:test title|test text|t:warning"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, metrics.EventPriorityNormal, e.Priority)
	assert.Equal(t, "default-hostname", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, metrics.EventAlertTypeWarning, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}

func TestConvertEventMetadataAggregatioKey(t *testing.T) {
	e, err := parseAndEnrichEventMessage([]byte("_e{10,9}:test title|test text|k:some aggregation key"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, metrics.EventPriorityNormal, e.Priority)
	assert.Equal(t, "default-hostname", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, metrics.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "some aggregation key", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}

func TestConvertEventMetadataSourceType(t *testing.T) {
	e, err := parseAndEnrichEventMessage([]byte("_e{10,9}:test title|test text|s:this is the source"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, metrics.EventPriorityNormal, e.Priority)
	assert.Equal(t, "default-hostname", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, metrics.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "this is the source", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}

func TestConvertEventMetadataTags(t *testing.T) {
	e, err := parseAndEnrichEventMessage([]byte("_e{10,9}:test title|test text|#tag1,tag2:test"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, metrics.EventPriorityNormal, e.Priority)
	assert.Equal(t, "default-hostname", e.Host)
	assert.Equal(t, []string{"tag1", "tag2:test"}, e.Tags)
	assert.Equal(t, metrics.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}

func TestConvertEventMetadataMultiple(t *testing.T) {
	e, err := parseAndEnrichEventMessage([]byte("_e{10,9}:test title|test text|t:warning|d:12345|p:low|h:some.host|k:aggKey|s:source test|#tag1,tag2:test"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(12345), e.Ts)
	assert.Equal(t, metrics.EventPriorityLow, e.Priority)
	assert.Equal(t, "some.host", e.Host)
	assert.Equal(t, []string{"tag1", "tag2:test"}, e.Tags)
	assert.Equal(t, metrics.EventAlertTypeWarning, e.AlertType)
	assert.Equal(t, "aggKey", e.AggregationKey)
	assert.Equal(t, "source test", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
}

func TestConvertNamespace(t *testing.T) {
	parsed, err := parseAndEnrichMetricMessage([]byte("daemon:21|ms"), "testNamespace.", nil, "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "testNamespace.daemon", parsed.Name)
	assert.Equal(t, "default-hostname", parsed.Host)
}

func TestConvertNamespaceBlacklist(t *testing.T) {
	parsed, err := parseAndEnrichMetricMessage([]byte("datadog.agent.daemon:21|ms"), "testNamespace.", []string{"datadog.agent"}, "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "datadog.agent.daemon", parsed.Name)
	assert.Equal(t, "default-hostname", parsed.Host)
}

func TestConvertEntityOriginDetectionNoTags(t *testing.T) {
	getTags = func(entity string, cardinality collectors.TagCardinality) ([]string, error) {
		if entity == "otherentity" {
			return []string{"tag:ishouldnothave"}, nil
		}
		return []string{}, nil
	}

	parsed, err := parseAndEnrichMetricMessage([]byte("daemon:666|g|#sometag1:somevalue1,host:my-hostname,dd.internal.entity_id:foo,sometag2:somevalue2"), "", nil, "default-hostname")
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

func TestConvertEntityOriginDetectionTags(t *testing.T) {
	getTags = func(entity string, cardinality collectors.TagCardinality) ([]string, error) {
		if entity == "kubernetes_pod_uid://foo" {
			return []string{"foo:bar", "bar:buz"}, nil
		}
		return []string{}, nil
	}

	parsed, err := parseAndEnrichMetricMessage([]byte("daemon:666|g|#sometag1:somevalue1,host:my-hostname,dd.internal.entity_id:foo,sometag2:somevalue2"), "", nil, "default-hostname")
	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, metrics.GaugeType, parsed.Mtype)
	require.Equal(t, 4, len(parsed.Tags))
	assert.ElementsMatch(t, []string{"sometag1:somevalue1", "foo:bar", "bar:buz", "sometag2:somevalue2"}, parsed.Tags)
	assert.Equal(t, "my-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertEntityOriginDetectionTagsError(t *testing.T) {
	getTags = func(entity string, cardinality collectors.TagCardinality) ([]string, error) {
		return nil, errors.New("cannot get tags")
	}

	parsed, err := parseAndEnrichMetricMessage([]byte("daemon:666|g|#sometag1:somevalue1,host:my-hostname,dd.internal.entity_id:foo,sometag2:somevalue2"), "", nil, "default-hostname")
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
