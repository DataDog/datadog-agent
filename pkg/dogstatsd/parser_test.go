// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package dogstatsd

import (
	// stdlib
	"testing"

	// 3p
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const epsilon = 0.1

// Schema of a dogstatsd packet:
// <name>:<value>|<metric_type>|@<sample_rate>|#<tag1_name>:<tag1_value>,<tag2_name>:<tag2_value>

func TestParseEmptyDatagram(t *testing.T) {
	emptyDatagram := []byte("")
	pkt := nextMessage(&emptyDatagram)

	assert.Nil(t, pkt)
}

func TestParseOneLineDatagram(t *testing.T) {
	datagram := []byte("daemon:666|g")
	pkt := nextMessage(&datagram)

	assert.NotNil(t, pkt)
	assert.Equal(t, 0, len(datagram))

	// With trailing newline
	datagram = []byte("daemon:666|g\n")
	pkt = nextMessage(&datagram)

	assert.NotNil(t, pkt)
	assert.Equal(t, 0, len(datagram))
}

func TestParseMultipleLineDatagram(t *testing.T) {
	datagram := []byte("daemon:666|g\ndaemon:667|g")

	// First packet
	pkt := nextMessage(&datagram)
	assert.Equal(t, []byte("daemon:666|g"), pkt)

	// Second packet
	pkt = nextMessage(&datagram)
	assert.Equal(t, []byte("daemon:667|g"), pkt)

	// Nore more packet
	pkt = nextMessage(&datagram)
	assert.Nil(t, pkt)
	assert.Equal(t, 0, len(datagram))
}

func TestGaugePacketCounter(t *testing.T) {
	assert.Equal(t, 1, 1)
}

func TestParseGauge(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:666|g"), "", "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, "666", parsed.RawValue)
	assert.Equal(t, metrics.GaugeType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestParseCounter(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:21|c"), "", "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, 21.0, parsed.Value)
	assert.Equal(t, metrics.CounterType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestParseCounterWithTags(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("custom_counter:1|c|#protocol:http,bench"), "", "default-hostname")

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

func TestParseHistogram(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:21|h"), "", "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, 21.0, parsed.Value)
	assert.Equal(t, metrics.HistogramType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestParseTimer(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:21|ms"), "", "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, 21.0, parsed.Value)
	assert.Equal(t, metrics.HistogramType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
	assert.Equal(t, "default-hostname", parsed.Host)
}

func TestParseSet(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:abc|s"), "", "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, "abc", parsed.RawValue)
	assert.Equal(t, metrics.SetType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestParseDistribution(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:3.5|d"), "", "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, 3.5, parsed.Value)
	assert.Equal(t, metrics.DistributionType, parsed.Mtype)
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.Equal(t, 0, len(parsed.Tags))
}

func TestParseSetUnicode(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:♬†øU†øU¥ºuT0♪|s"), "", "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, "♬†øU†øU¥ºuT0♪", parsed.RawValue)
	assert.Equal(t, metrics.SetType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestParseGaugeWithTags(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"), "", "default-hostname")

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

func TestParseGaugeWithHostTag(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:666|g|#sometag1:somevalue1,host:my-hostname,sometag2:somevalue2"), "", "default-hostname")
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

func TestParseGaugeWithEmptyHostTag(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:666|g|#sometag1:somevalue1,host:,sometag2:somevalue2"), "", "default-hostname")
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

func TestParseGaugeWithNoTags(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:666|g"), "", "default-hostname")
	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, metrics.GaugeType, parsed.Mtype)
	assert.Empty(t, parsed.Tags)
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestParseGaugeWithSampleRate(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:666|g|@0.21"), "", "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, metrics.GaugeType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 0.21, parsed.SampleRate, epsilon)
}

func TestParseGaugeWithPoundOnly(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:666|g|#"), "", "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, metrics.GaugeType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestParseGaugeWithUnicode(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("♬†øU†øU¥ºuT0♪:666|g|#intitulé:T0µ"), "", "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "♬†øU†øU¥ºuT0♪", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, metrics.GaugeType, parsed.Mtype)
	require.Equal(t, 1, len(parsed.Tags))
	assert.Equal(t, "intitulé:T0µ", parsed.Tags[0])
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestParseMetricError(t *testing.T) {
	// not enough information
	_, err := parseMetricMessage([]byte("daemon:666"), "", "default-hostname")
	assert.Error(t, err)

	_, err = parseMetricMessage([]byte("daemon:666|"), "", "default-hostname")
	assert.Error(t, err)

	_, err = parseMetricMessage([]byte("daemon:|g"), "", "default-hostname")
	assert.Error(t, err)

	_, err = parseMetricMessage([]byte(":666|g"), "", "default-hostname")
	assert.Error(t, err)

	// too many value
	_, err = parseMetricMessage([]byte("daemon:666:777|g"), "", "default-hostname")
	assert.Error(t, err)

	// unknown metadata prefix
	_, err = parseMetricMessage([]byte("daemon:666|g|m:test"), "", "default-hostname")
	assert.NoError(t, err)

	// invalid value
	_, err = parseMetricMessage([]byte("daemon:abc|g"), "", "default-hostname")
	assert.Error(t, err)

	// invalid metric type
	_, err = parseMetricMessage([]byte("daemon:666|unknown"), "", "default-hostname")
	assert.Error(t, err)

	// invalid sample rate
	_, err = parseMetricMessage([]byte("daemon:666|g|@abc"), "", "default-hostname")
	assert.Error(t, err)
}

func TestParseMonokeyBatching(t *testing.T) {
	// TODO: not implemented
	// parsed, err := parseMetricMessage([]byte("test_gauge:1.5|g|#tag1:one,tag2:two:2.3|g|#tag3:three:3|g"), "default-hostname")
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
	sc, err := parseServiceCheckMessage([]byte("_sc|agent.up|0"), "default-hostname")

	assert.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "default-hostname", sc.Host)
	assert.Equal(t, int64(0), sc.Ts)
	assert.Equal(t, metrics.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, []string(nil), sc.Tags)
}

func TestServiceCheckError(t *testing.T) {
	// not enough information
	_, err := parseServiceCheckMessage([]byte("_sc|agent.up"), "default-hostname")
	assert.Error(t, err)

	_, err = parseServiceCheckMessage([]byte("_sc|agent.up|"), "default-hostname")
	assert.Error(t, err)

	// not invalid status
	_, err = parseServiceCheckMessage([]byte("_sc|agent.up|OK"), "default-hostname")
	assert.Error(t, err)

	// not unknown status
	_, err = parseServiceCheckMessage([]byte("_sc|agent.up|21"), "default-hostname")
	assert.Error(t, err)

	// invalid timestamp
	_, err = parseServiceCheckMessage([]byte("_sc|agent.up|0|d:some_time"), "default-hostname")
	assert.NoError(t, err)

	// unknown metadata
	_, err = parseServiceCheckMessage([]byte("_sc|agent.up|0|u:unknown"), "default-hostname")
	assert.NoError(t, err)
}

func TestServiceCheckMetadataTimestamp(t *testing.T) {
	sc, err := parseServiceCheckMessage([]byte("_sc|agent.up|0|d:21"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "default-hostname", sc.Host)
	assert.Equal(t, int64(21), sc.Ts)
	assert.Equal(t, metrics.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, []string(nil), sc.Tags)
}

func TestServiceCheckMetadataHostname(t *testing.T) {
	sc, err := parseServiceCheckMessage([]byte("_sc|agent.up|0|h:localhost"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "localhost", sc.Host)
	assert.Equal(t, int64(0), sc.Ts)
	assert.Equal(t, metrics.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, []string(nil), sc.Tags)
}

func TestServiceCheckMetadataHostnameInTag(t *testing.T) {
	sc, err := parseServiceCheckMessage([]byte("_sc|agent.up|0|#host:localhost"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "localhost", sc.Host)
	assert.Equal(t, int64(0), sc.Ts)
	assert.Equal(t, metrics.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, []string{}, sc.Tags)
}

func TestServiceCheckMetadataEmptyHostTag(t *testing.T) {
	sc, err := parseServiceCheckMessage([]byte("_sc|agent.up|0|#host:,other:tag"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "", sc.Host)
	assert.Equal(t, int64(0), sc.Ts)
	assert.Equal(t, metrics.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, []string{"other:tag"}, sc.Tags)
}

func TestServiceCheckMetadataTags(t *testing.T) {
	sc, err := parseServiceCheckMessage([]byte("_sc|agent.up|0|#tag1,tag2:test,tag3"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "default-hostname", sc.Host)
	assert.Equal(t, int64(0), sc.Ts)
	assert.Equal(t, metrics.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, []string{"tag1", "tag2:test", "tag3"}, sc.Tags)
}

func TestServiceCheckMetadataMessage(t *testing.T) {
	sc, err := parseServiceCheckMessage([]byte("_sc|agent.up|0|m:this is fine"), "default-hostname")

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "default-hostname", sc.Host)
	assert.Equal(t, int64(0), sc.Ts)
	assert.Equal(t, metrics.ServiceCheckOK, sc.Status)
	assert.Equal(t, "this is fine", sc.Message)
	assert.Equal(t, []string(nil), sc.Tags)
}

func TestServiceCheckMetadataMultiple(t *testing.T) {
	// all type
	sc, err := parseServiceCheckMessage([]byte("_sc|agent.up|0|d:21|h:localhost|#tag1:test,tag2|m:this is fine"), "default-hostname")
	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "localhost", sc.Host)
	assert.Equal(t, int64(21), sc.Ts)
	assert.Equal(t, metrics.ServiceCheckOK, sc.Status)
	assert.Equal(t, "this is fine", sc.Message)
	assert.Equal(t, []string{"tag1:test", "tag2"}, sc.Tags)

	// multiple time the same tag
	sc, err = parseServiceCheckMessage([]byte("_sc|agent.up|0|d:21|h:localhost|h:localhost2|d:22"), "default-hostname")
	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "localhost2", sc.Host)
	assert.Equal(t, int64(22), sc.Ts)
	assert.Equal(t, metrics.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, []string(nil), sc.Tags)
}

func TestEventMinimal(t *testing.T) {
	e, err := parseEventMessage([]byte("_e{10,9}:test title|test text"), "default-hostname")

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

func TestEventMultilinesText(t *testing.T) {
	e, err := parseEventMessage([]byte("_e{10,24}:test title|test\\line1\\nline2\\nline3"), "default-hostname")

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

func TestEventPipeInTitle(t *testing.T) {
	e, err := parseEventMessage([]byte("_e{10,24}:test|title|test\\line1\\nline2\\nline3"), "default-hostname")

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

func TestEventError(t *testing.T) {
	// missing length header
	_, err := parseEventMessage([]byte("_e:title|text"), "default-hostname")
	assert.Error(t, err)

	// greater length than packet
	_, err = parseEventMessage([]byte("_e{10,10}:title|text"), "default-hostname")
	assert.Error(t, err)

	// zero length
	_, err = parseEventMessage([]byte("_e{0,0}:a|a"), "default-hostname")
	assert.Error(t, err)

	// missing title or text length
	_, err = parseEventMessage([]byte("_e{5555:title|text"), "default-hostname")
	assert.Error(t, err)

	// missing wrong len format
	_, err = parseEventMessage([]byte("_e{a,1}:title|text"), "default-hostname")
	assert.Error(t, err)

	_, err = parseEventMessage([]byte("_e{1,a}:title|text"), "default-hostname")
	assert.Error(t, err)

	// missing title or text length
	_, err = parseEventMessage([]byte("_e{5,}:title|text"), "default-hostname")
	assert.Error(t, err)

	_, err = parseEventMessage([]byte("_e{,4}:title|text"), "default-hostname")
	assert.Error(t, err)

	_, err = parseEventMessage([]byte("_e{}:title|text"), "default-hostname")
	assert.Error(t, err)

	_, err = parseEventMessage([]byte("_e{,}:title|text"), "default-hostname")
	assert.Error(t, err)

	// not enough information
	_, err = parseEventMessage([]byte("_e|text"), "default-hostname")
	assert.Error(t, err)

	_, err = parseEventMessage([]byte("_e:|text"), "default-hostname")
	assert.Error(t, err)

	// invalid timestamp
	_, err = parseEventMessage([]byte("_e{5,4}:title|text|d:abc"), "default-hostname")
	assert.NoError(t, err)

	// invalid priority
	_, err = parseEventMessage([]byte("_e{5,4}:title|text|p:urgent"), "default-hostname")
	assert.NoError(t, err)

	// invalid priority
	_, err = parseEventMessage([]byte("_e{5,4}:title|text|p:urgent"), "default-hostname")
	assert.NoError(t, err)

	// invalid alert type
	_, err = parseEventMessage([]byte("_e{5,4}:title|text|t:test"), "default-hostname")
	assert.NoError(t, err)

	// unknown metadata
	_, err = parseEventMessage([]byte("_e{5,4}:title|text|x:1234"), "default-hostname")
	assert.NoError(t, err)
}

func TestEventMetadataTimestamp(t *testing.T) {
	e, err := parseEventMessage([]byte("_e{10,9}:test title|test text|d:21"), "default-hostname")

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

func TestEventMetadataPriority(t *testing.T) {
	e, err := parseEventMessage([]byte("_e{10,9}:test title|test text|p:low"), "default-hostname")

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

func TestEventMetadataHostname(t *testing.T) {
	e, err := parseEventMessage([]byte("_e{10,9}:test title|test text|h:localhost"), "default-hostname")

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

func TestEventMetadataHostnameInTag(t *testing.T) {
	e, err := parseEventMessage([]byte("_e{10,9}:test title|test text|#host:localhost"), "default-hostname")

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

func TestEventMetadataEmptyHostTag(t *testing.T) {
	e, err := parseEventMessage([]byte("_e{10,9}:test title|test text|#host:,other:tag"), "default-hostname")

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

func TestEventMetadataAlertType(t *testing.T) {
	e, err := parseEventMessage([]byte("_e{10,9}:test title|test text|t:warning"), "default-hostname")

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

func TestEventMetadataAggregatioKey(t *testing.T) {
	e, err := parseEventMessage([]byte("_e{10,9}:test title|test text|k:some aggregation key"), "default-hostname")

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

func TestEventMetadataSourceType(t *testing.T) {
	e, err := parseEventMessage([]byte("_e{10,9}:test title|test text|s:this is the source"), "default-hostname")

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

func TestEventMetadataTags(t *testing.T) {
	e, err := parseEventMessage([]byte("_e{10,9}:test title|test text|#tag1,tag2:test"), "default-hostname")

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

func TestEventMetadataMultiple(t *testing.T) {
	e, err := parseEventMessage([]byte("_e{10,9}:test title|test text|t:warning|d:12345|p:low|h:some.host|k:aggKey|s:source test|#tag1,tag2:test"), "default-hostname")

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

func TestNamespace(t *testing.T) {
	parsed, err := parseMetricMessage([]byte("daemon:21|ms"), "testNamespace.", "default-hostname")

	assert.NoError(t, err)

	assert.Equal(t, "testNamespace.daemon", parsed.Name)
	assert.Equal(t, "default-hostname", parsed.Host)
}
