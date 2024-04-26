// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	// All types except set
	symbolToType = map[string]metrics.MetricType{
		"g":  metrics.GaugeType,
		"c":  metrics.CounterType,
		"h":  metrics.HistogramType,
		"ms": metrics.HistogramType,
		"d":  metrics.DistributionType,
	}
)

func parseAndEnrichSingleMetricMessage(t *testing.T, message []byte, conf enrichConfig) (metrics.MetricSample, error) {
	deps := newServerDeps(t)
	parser := newParser(deps.Config, newFloat64ListPool(), 1, deps.WMeta)
	parsed, err := parser.parseMetricSample(message)
	if err != nil {
		return metrics.MetricSample{}, err
	}

	samples := []metrics.MetricSample{}
	samples = enrichMetricSample(samples, parsed, "", "", conf)
	if len(samples) != 1 {
		return metrics.MetricSample{}, fmt.Errorf("wrong number of metrics parsed")
	}
	return samples[0], nil
}

func parseAndEnrichMultipleMetricMessage(t *testing.T, message []byte, conf enrichConfig) ([]metrics.MetricSample, error) {
	deps := newServerDeps(t)
	parser := newParser(deps.Config, newFloat64ListPool(), 1, deps.WMeta)
	parsed, err := parser.parseMetricSample(message)
	if err != nil {
		return []metrics.MetricSample{}, err
	}

	samples := []metrics.MetricSample{}
	return enrichMetricSample(samples, parsed, "", "", conf), nil
}

func parseAndEnrichServiceCheckMessage(t *testing.T, message []byte, conf enrichConfig) (*servicecheck.ServiceCheck, error) {
	deps := newServerDeps(t)
	parser := newParser(deps.Config, newFloat64ListPool(), 1, deps.WMeta)
	parsed, err := parser.parseServiceCheck(message)
	if err != nil {
		return nil, err
	}
	return enrichServiceCheck(parsed, "", conf), nil
}

func parseAndEnrichEventMessage(t *testing.T, message []byte, conf enrichConfig) (*event.Event, error) {
	deps := newServerDeps(t)
	parser := newParser(deps.Config, newFloat64ListPool(), 1, deps.WMeta)
	parsed, err := parser.parseEvent(message)
	if err != nil {
		return nil, err
	}
	return enrichEvent(parsed, "", conf), nil
}

func TestConvertParseMultiple(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}

	for metricSymbol, metricType := range symbolToType {

		parsed, err := parseAndEnrichMultipleMetricMessage(t, []byte("daemon:666:777.5|"+metricSymbol), conf)
		assert.NoError(t, err)
		require.Len(t, parsed, 2)

		assert.Equal(t, "daemon", parsed[0].Name)
		assert.InEpsilon(t, 666.0, parsed[0].Value, epsilon)
		assert.Equal(t, metricType, parsed[0].Mtype)
		assert.Equal(t, 0, len(parsed[0].Tags))
		assert.Equal(t, "default-hostname", parsed[0].Host)
		assert.Equal(t, "", parsed[0].OriginInfo.FromUDS)
		assert.Equal(t, "", parsed[0].OriginInfo.FromTag)
		assert.Equal(t, "", parsed[0].OriginInfo.FromMsg)
		assert.InEpsilon(t, 1.0, parsed[0].SampleRate, epsilon)

		assert.Equal(t, "daemon", parsed[1].Name)
		assert.InEpsilon(t, 777.5, parsed[1].Value, epsilon)
		assert.Equal(t, metricType, parsed[1].Mtype)
		assert.Equal(t, 0, len(parsed[1].Tags))
		assert.Equal(t, "default-hostname", parsed[1].Host)
		assert.Equal(t, "", parsed[0].OriginInfo.FromUDS)
		assert.Equal(t, "", parsed[0].OriginInfo.FromTag)
		assert.Equal(t, "", parsed[0].OriginInfo.FromMsg)
		assert.InEpsilon(t, 1.0, parsed[1].SampleRate, epsilon)
	}
}

func TestConvertParseSingle(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}

	for metricSymbol, metricType := range symbolToType {

		parsed, err := parseAndEnrichMultipleMetricMessage(t, []byte("daemon:666|"+metricSymbol), conf)

		assert.NoError(t, err)
		require.Len(t, parsed, 1)

		assert.Equal(t, "daemon", parsed[0].Name)
		assert.InEpsilon(t, 666.0, parsed[0].Value, epsilon)
		assert.Equal(t, metricType, parsed[0].Mtype)
		assert.Equal(t, 0, len(parsed[0].Tags))
		assert.Equal(t, "default-hostname", parsed[0].Host)
		assert.Equal(t, "", parsed[0].OriginInfo.FromUDS)
		assert.Equal(t, "", parsed[0].OriginInfo.FromTag)
		assert.Equal(t, "", parsed[0].OriginInfo.FromMsg)
		assert.InEpsilon(t, 1.0, parsed[0].SampleRate, epsilon)
	}
}

func TestConvertParseSingleWithTags(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}

	for metricSymbol, metricType := range symbolToType {

		parsed, err := parseAndEnrichMultipleMetricMessage(t, []byte("daemon:666|"+metricSymbol+"|#protocol:http,bench"), conf)

		assert.NoError(t, err)
		require.Len(t, parsed, 1)

		assert.Equal(t, "daemon", parsed[0].Name)
		assert.InEpsilon(t, 666.0, parsed[0].Value, epsilon)
		assert.Equal(t, metricType, parsed[0].Mtype)
		assert.Equal(t, 2, len(parsed[0].Tags))
		assert.Equal(t, "protocol:http", parsed[0].Tags[0])
		assert.Equal(t, "bench", parsed[0].Tags[1])
		assert.Equal(t, "default-hostname", parsed[0].Host)
		assert.Equal(t, "", parsed[0].OriginInfo.FromUDS)
		assert.Equal(t, "", parsed[0].OriginInfo.FromTag)
		assert.Equal(t, "", parsed[0].OriginInfo.FromMsg)
		assert.InEpsilon(t, 1.0, parsed[0].SampleRate, epsilon)
	}
}

func TestConvertParseSingleWithHostTags(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}

	for metricSymbol, metricType := range symbolToType {

		parsed, err := parseAndEnrichMultipleMetricMessage(t, []byte("daemon:666|"+metricSymbol+"|#protocol:http,host:custom-host,bench"), conf)

		assert.NoError(t, err)
		require.Len(t, parsed, 1)

		assert.Equal(t, "daemon", parsed[0].Name)
		assert.InEpsilon(t, 666.0, parsed[0].Value, epsilon)
		assert.Equal(t, metricType, parsed[0].Mtype)
		assert.Equal(t, 2, len(parsed[0].Tags))
		assert.Equal(t, "protocol:http", parsed[0].Tags[0])
		assert.Equal(t, "bench", parsed[0].Tags[1])
		assert.Equal(t, "custom-host", parsed[0].Host)
		assert.Equal(t, "", parsed[0].OriginInfo.FromUDS)
		assert.Equal(t, "", parsed[0].OriginInfo.FromTag)
		assert.Equal(t, "", parsed[0].OriginInfo.FromMsg)
		assert.InEpsilon(t, 1.0, parsed[0].SampleRate, epsilon)
	}
}

func TestConvertParseSingleWithEmptyHostTags(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}

	for metricSymbol, metricType := range symbolToType {

		parsed, err := parseAndEnrichMultipleMetricMessage(t, []byte("daemon:666|"+metricSymbol+"|#protocol:http,host:,bench"), conf)

		assert.NoError(t, err)
		require.Len(t, parsed, 1)

		assert.Equal(t, "daemon", parsed[0].Name)
		assert.InEpsilon(t, 666.0, parsed[0].Value, epsilon)
		assert.Equal(t, metricType, parsed[0].Mtype)
		assert.Equal(t, 2, len(parsed[0].Tags))
		assert.Equal(t, "protocol:http", parsed[0].Tags[0])
		assert.Equal(t, "bench", parsed[0].Tags[1])
		assert.Equal(t, "", parsed[0].Host)
		assert.Equal(t, "", parsed[0].OriginInfo.FromUDS)
		assert.Equal(t, "", parsed[0].OriginInfo.FromTag)
		assert.Equal(t, "", parsed[0].OriginInfo.FromMsg)
		assert.InEpsilon(t, 1.0, parsed[0].SampleRate, epsilon)
	}
}

func TestConvertParseSingleWithSampleRate(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}

	for metricSymbol, metricType := range symbolToType {

		parsed, err := parseAndEnrichMultipleMetricMessage(t, []byte("daemon:666|"+metricSymbol+"|@0.21"), conf)

		assert.NoError(t, err)
		require.Len(t, parsed, 1)

		assert.Equal(t, "daemon", parsed[0].Name)
		assert.InEpsilon(t, 666.0, parsed[0].Value, epsilon)
		assert.Equal(t, metricType, parsed[0].Mtype)
		assert.Equal(t, 0, len(parsed[0].Tags))
		assert.Equal(t, "default-hostname", parsed[0].Host)
		assert.Equal(t, "", parsed[0].OriginInfo.FromUDS)
		assert.Equal(t, "", parsed[0].OriginInfo.FromTag)
		assert.Equal(t, "", parsed[0].OriginInfo.FromMsg)
		assert.InEpsilon(t, 0.21, parsed[0].SampleRate, epsilon)
	}
}

func TestConvertParseSet(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}

	parsed, err := parseAndEnrichSingleMetricMessage(t, []byte("daemon:abc:def|s"), conf)

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, "abc:def", parsed.RawValue)
	assert.Equal(t, metrics.SetType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.Equal(t, "", parsed.OriginInfo.FromUDS)
	assert.Equal(t, "", parsed.OriginInfo.FromTag)
	assert.Equal(t, "", parsed.OriginInfo.FromMsg)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertParseSetUnicode(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}

	parsed, err := parseAndEnrichSingleMetricMessage(t, []byte("daemon:♬†øU†øU¥ºuT0♪|s"), conf)

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, "♬†øU†øU¥ºuT0♪", parsed.RawValue)
	assert.Equal(t, metrics.SetType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.Equal(t, "", parsed.OriginInfo.FromUDS)
	assert.Equal(t, "", parsed.OriginInfo.FromTag)
	assert.Equal(t, "", parsed.OriginInfo.FromMsg)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertParseGaugeWithPoundOnly(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}

	parsed, err := parseAndEnrichSingleMetricMessage(t, []byte("daemon:666|g|#"), conf)

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, metrics.GaugeType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.Equal(t, "", parsed.OriginInfo.FromUDS)
	assert.Equal(t, "", parsed.OriginInfo.FromTag)
	assert.Equal(t, "", parsed.OriginInfo.FromMsg)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertParseGaugeWithUnicode(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}

	parsed, err := parseAndEnrichSingleMetricMessage(t, []byte("♬†øU†øU¥ºuT0♪:666|g|#intitulé:T0µ"), conf)

	assert.NoError(t, err)

	assert.Equal(t, "♬†øU†øU¥ºuT0♪", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, metrics.GaugeType, parsed.Mtype)
	require.Equal(t, 1, len(parsed.Tags))
	assert.Equal(t, "intitulé:T0µ", parsed.Tags[0])
	assert.Equal(t, "default-hostname", parsed.Host)
	assert.Equal(t, "", parsed.OriginInfo.FromUDS)
	assert.Equal(t, "", parsed.OriginInfo.FromTag)
	assert.Equal(t, "", parsed.OriginInfo.FromMsg)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertParseMetricError(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}

	// not enough information
	_, err := parseAndEnrichSingleMetricMessage(t, []byte("daemon:666"), conf)
	assert.Error(t, err)

	_, err = parseAndEnrichSingleMetricMessage(t, []byte("daemon:666|"), conf)
	assert.Error(t, err)

	_, err = parseAndEnrichSingleMetricMessage(t, []byte("daemon:|g"), conf)
	assert.Error(t, err)

	_, err = parseAndEnrichSingleMetricMessage(t, []byte(":666|g"), conf)
	assert.Error(t, err)

	// unknown metadata prefix
	_, err = parseAndEnrichSingleMetricMessage(t, []byte("daemon:666|g|m:test"), conf)
	assert.NoError(t, err)

	// invalid value
	_, err = parseAndEnrichSingleMetricMessage(t, []byte("daemon:abc|g"), conf)
	assert.Error(t, err)

	// invalid metric type
	_, err = parseAndEnrichSingleMetricMessage(t, []byte("daemon:666|unknown"), conf)
	assert.Error(t, err)

	// invalid sample rate
	_, err = parseAndEnrichSingleMetricMessage(t, []byte("daemon:666|g|@abc"), conf)
	assert.Error(t, err)
}

//nolint:revive // TODO(AML) Fix revive linter
func TestConvertParseMonokeyBatching(t *testing.T) {
	// TODO: not implemented
	// parsed, err := parseAndEnrichSingleMetricMessage(t, []byte("test_gauge:1.5|g|#tag1:one,tag2:two:2.3|g|#tag3:three:3|g"), "default-hostname")
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
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}

	sc, err := parseAndEnrichServiceCheckMessage(t, []byte("_sc|agent.up|0"), conf)

	assert.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "default-hostname", sc.Host)
	assert.Equal(t, int64(0), sc.Ts)
	assert.Equal(t, servicecheck.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, "", sc.OriginInfo.FromUDS)
	assert.Equal(t, "", sc.OriginInfo.FromTag)
	assert.Equal(t, "", sc.OriginInfo.FromMsg)
	assert.Equal(t, []string(nil), sc.Tags)
}

func TestConvertServiceCheckError(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}

	// not enough information
	_, err := parseAndEnrichServiceCheckMessage(t, []byte("_sc|agent.up"), conf)
	assert.Error(t, err)

	_, err = parseAndEnrichServiceCheckMessage(t, []byte("_sc|agent.up|"), conf)
	assert.Error(t, err)

	// not invalid status
	_, err = parseAndEnrichServiceCheckMessage(t, []byte("_sc|agent.up|OK"), conf)
	assert.Error(t, err)

	// not unknown status
	_, err = parseAndEnrichServiceCheckMessage(t, []byte("_sc|agent.up|21"), conf)
	assert.Error(t, err)

	// invalid timestamp
	_, err = parseAndEnrichServiceCheckMessage(t, []byte("_sc|agent.up|0|d:some_time"), conf)
	assert.NoError(t, err)

	// unknown metadata
	_, err = parseAndEnrichServiceCheckMessage(t, []byte("_sc|agent.up|0|u:unknown"), conf)
	assert.NoError(t, err)
}

func TestConvertServiceCheckMetadataTimestamp(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	sc, err := parseAndEnrichServiceCheckMessage(t, []byte("_sc|agent.up|0|d:21"), conf)

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "default-hostname", sc.Host)
	assert.Equal(t, int64(21), sc.Ts)
	assert.Equal(t, servicecheck.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, "", sc.OriginInfo.FromUDS)
	assert.Equal(t, "", sc.OriginInfo.FromTag)
	assert.Equal(t, "", sc.OriginInfo.FromMsg)
	assert.Equal(t, []string(nil), sc.Tags)
}

func TestConvertServiceCheckMetadataHostname(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	sc, err := parseAndEnrichServiceCheckMessage(t, []byte("_sc|agent.up|0|h:localhost"), conf)

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "localhost", sc.Host)
	assert.Equal(t, int64(0), sc.Ts)
	assert.Equal(t, servicecheck.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, "", sc.OriginInfo.FromUDS)
	assert.Equal(t, "", sc.OriginInfo.FromTag)
	assert.Equal(t, "", sc.OriginInfo.FromMsg)
	assert.Equal(t, []string(nil), sc.Tags)
}

func TestConvertServiceCheckMetadataHostnameInTag(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	sc, err := parseAndEnrichServiceCheckMessage(t, []byte("_sc|agent.up|0|#host:localhost"), conf)

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "localhost", sc.Host)
	assert.Equal(t, int64(0), sc.Ts)
	assert.Equal(t, servicecheck.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, "", sc.OriginInfo.FromUDS)
	assert.Equal(t, "", sc.OriginInfo.FromTag)
	assert.Equal(t, "", sc.OriginInfo.FromMsg)
	assert.Equal(t, []string{}, sc.Tags)
}

func TestConvertServiceCheckMetadataEmptyHostTag(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	sc, err := parseAndEnrichServiceCheckMessage(t, []byte("_sc|agent.up|0|#host:,other:tag"), conf)

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "", sc.Host)
	assert.Equal(t, int64(0), sc.Ts)
	assert.Equal(t, servicecheck.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, "", sc.OriginInfo.FromUDS)
	assert.Equal(t, "", sc.OriginInfo.FromTag)
	assert.Equal(t, "", sc.OriginInfo.FromMsg)
	assert.Equal(t, []string{"other:tag"}, sc.Tags)
}

func TestConvertServiceCheckMetadataTags(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	sc, err := parseAndEnrichServiceCheckMessage(t, []byte("_sc|agent.up|0|#tag1,tag2:test,tag3"), conf)

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "default-hostname", sc.Host)
	assert.Equal(t, int64(0), sc.Ts)
	assert.Equal(t, servicecheck.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, "", sc.OriginInfo.FromUDS)
	assert.Equal(t, "", sc.OriginInfo.FromTag)
	assert.Equal(t, "", sc.OriginInfo.FromMsg)
	assert.Equal(t, []string{"tag1", "tag2:test", "tag3"}, sc.Tags)
}

func TestConvertServiceCheckMetadataMessage(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	sc, err := parseAndEnrichServiceCheckMessage(t, []byte("_sc|agent.up|0|m:this is fine"), conf)

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "default-hostname", sc.Host)
	assert.Equal(t, int64(0), sc.Ts)
	assert.Equal(t, servicecheck.ServiceCheckOK, sc.Status)
	assert.Equal(t, "this is fine", sc.Message)
	assert.Equal(t, "", sc.OriginInfo.FromUDS)
	assert.Equal(t, "", sc.OriginInfo.FromTag)
	assert.Equal(t, "", sc.OriginInfo.FromMsg)
	assert.Equal(t, []string(nil), sc.Tags)
}

func TestConvertServiceCheckMetadataMultiple(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	// all type
	sc, err := parseAndEnrichServiceCheckMessage(t, []byte("_sc|agent.up|0|d:21|h:localhost|#tag1:test,tag2|m:this is fine"), conf)
	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "localhost", sc.Host)
	assert.Equal(t, int64(21), sc.Ts)
	assert.Equal(t, servicecheck.ServiceCheckOK, sc.Status)
	assert.Equal(t, "this is fine", sc.Message)
	assert.Equal(t, "", sc.OriginInfo.FromUDS)
	assert.Equal(t, "", sc.OriginInfo.FromTag)
	assert.Equal(t, "", sc.OriginInfo.FromMsg)
	assert.Equal(t, []string{"tag1:test", "tag2"}, sc.Tags)

	// multiple time the same tag
	sc, err = parseAndEnrichServiceCheckMessage(t, []byte("_sc|agent.up|0|d:21|h:localhost|h:localhost2|d:22"), conf)
	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "localhost2", sc.Host)
	assert.Equal(t, int64(22), sc.Ts)
	assert.Equal(t, servicecheck.ServiceCheckOK, sc.Status)
	assert.Equal(t, "", sc.Message)
	assert.Equal(t, "", sc.OriginInfo.FromUDS)
	assert.Equal(t, "", sc.OriginInfo.FromTag)
	assert.Equal(t, "", sc.OriginInfo.FromMsg)
	assert.Equal(t, []string(nil), sc.Tags)
}

func TestServiceCheckOriginTag(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	sc, err := parseAndEnrichServiceCheckMessage(t, []byte("_sc|agent.up|0|d:21|h:localhost|#tag1:test,tag2,dd.internal.entity_id:testID|m:this is fine"), conf)
	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.CheckName)
	assert.Equal(t, "localhost", sc.Host)
	assert.Equal(t, int64(21), sc.Ts)
	assert.Equal(t, servicecheck.ServiceCheckOK, sc.Status)
	assert.Equal(t, "this is fine", sc.Message)
	assert.Equal(t, "", sc.OriginInfo.FromUDS)
	assert.Equal(t, "testID", sc.OriginInfo.FromTag)
	assert.Equal(t, "", sc.OriginInfo.FromMsg)
	assert.Equal(t, []string{"tag1:test", "tag2"}, sc.Tags)
}

func TestConvertEventMinimal(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	e, err := parseAndEnrichEventMessage(t, []byte("_e{10,9}:test title|test text"), conf)

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, event.EventPriorityNormal, e.Priority)
	assert.Equal(t, "default-hostname", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, event.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
	assert.Equal(t, "", e.OriginInfo.FromUDS)
	assert.Equal(t, "", e.OriginInfo.FromTag)
	assert.Equal(t, "", e.OriginInfo.FromMsg)
}

func TestConvertEventMultilinesText(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	e, err := parseAndEnrichEventMessage(t, []byte("_e{10,24}:test title|test\\line1\\nline2\\nline3"), conf)

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test\\line1\nline2\nline3", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, event.EventPriorityNormal, e.Priority)
	assert.Equal(t, "default-hostname", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, event.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
	assert.Equal(t, "", e.OriginInfo.FromUDS)
	assert.Equal(t, "", e.OriginInfo.FromTag)
	assert.Equal(t, "", e.OriginInfo.FromMsg)
}

func TestConvertEventPipeInTitle(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	e, err := parseAndEnrichEventMessage(t, []byte("_e{10,24}:test|title|test\\line1\\nline2\\nline3"), conf)

	require.Nil(t, err)
	assert.Equal(t, "test|title", e.Title)
	assert.Equal(t, "test\\line1\nline2\nline3", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, event.EventPriorityNormal, e.Priority)
	assert.Equal(t, "default-hostname", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, event.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
	assert.Equal(t, "", e.OriginInfo.FromUDS)
	assert.Equal(t, "", e.OriginInfo.FromTag)
	assert.Equal(t, "", e.OriginInfo.FromMsg)
}

func TestConvertEventError(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	// missing length header
	_, err := parseAndEnrichEventMessage(t, []byte("_e:title|text"), conf)
	assert.Error(t, err)

	// greater length than packet
	_, err = parseAndEnrichEventMessage(t, []byte("_e{10,10}:title|text"), conf)
	assert.Error(t, err)

	// zero length
	_, err = parseAndEnrichEventMessage(t, []byte("_e{0,0}:a|a"), conf)
	assert.Error(t, err)

	// missing title or text length
	_, err = parseAndEnrichEventMessage(t, []byte("_e{5555:title|text"), conf)
	assert.Error(t, err)

	// missing wrong len format
	_, err = parseAndEnrichEventMessage(t, []byte("_e{a,1}:title|text"), conf)
	assert.Error(t, err)

	_, err = parseAndEnrichEventMessage(t, []byte("_e{1,a}:title|text"), conf)
	assert.Error(t, err)

	// missing title or text length
	_, err = parseAndEnrichEventMessage(t, []byte("_e{5,}:title|text"), conf)
	assert.Error(t, err)

	_, err = parseAndEnrichEventMessage(t, []byte("_e{,4}:title|text"), conf)
	assert.Error(t, err)

	_, err = parseAndEnrichEventMessage(t, []byte("_e{}:title|text"), conf)
	assert.Error(t, err)

	_, err = parseAndEnrichEventMessage(t, []byte("_e{,}:title|text"), conf)
	assert.Error(t, err)

	// not enough information
	_, err = parseAndEnrichEventMessage(t, []byte("_e|text"), conf)
	assert.Error(t, err)

	_, err = parseAndEnrichEventMessage(t, []byte("_e:|text"), conf)
	assert.Error(t, err)

	// invalid timestamp
	_, err = parseAndEnrichEventMessage(t, []byte("_e{5,4}:title|text|d:abc"), conf)
	assert.NoError(t, err)

	// invalid priority
	_, err = parseAndEnrichEventMessage(t, []byte("_e{5,4}:title|text|p:urgent"), conf)
	assert.NoError(t, err)

	// invalid priority
	_, err = parseAndEnrichEventMessage(t, []byte("_e{5,4}:title|text|p:urgent"), conf)
	assert.NoError(t, err)

	// invalid alert type
	_, err = parseAndEnrichEventMessage(t, []byte("_e{5,4}:title|text|t:test"), conf)
	assert.NoError(t, err)

	// unknown metadata
	_, err = parseAndEnrichEventMessage(t, []byte("_e{5,4}:title|text|x:1234"), conf)
	assert.NoError(t, err)
}

func TestConvertEventMetadataTimestamp(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	e, err := parseAndEnrichEventMessage(t, []byte("_e{10,9}:test title|test text|d:21"), conf)

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(21), e.Ts)
	assert.Equal(t, event.EventPriorityNormal, e.Priority)
	assert.Equal(t, "default-hostname", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, event.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
	assert.Equal(t, "", e.OriginInfo.FromUDS)
	assert.Equal(t, "", e.OriginInfo.FromTag)
	assert.Equal(t, "", e.OriginInfo.FromMsg)
}

func TestConvertEventMetadataPriority(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	e, err := parseAndEnrichEventMessage(t, []byte("_e{10,9}:test title|test text|p:low"), conf)

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, event.EventPriorityLow, e.Priority)
	assert.Equal(t, "default-hostname", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, event.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
	assert.Equal(t, "", e.OriginInfo.FromUDS)
	assert.Equal(t, "", e.OriginInfo.FromTag)
	assert.Equal(t, "", e.OriginInfo.FromMsg)
}

func TestConvertEventMetadataHostname(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	e, err := parseAndEnrichEventMessage(t, []byte("_e{10,9}:test title|test text|h:localhost"), conf)

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, event.EventPriorityNormal, e.Priority)
	assert.Equal(t, "localhost", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, event.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
	assert.Equal(t, "", e.OriginInfo.FromUDS)
	assert.Equal(t, "", e.OriginInfo.FromTag)
	assert.Equal(t, "", e.OriginInfo.FromMsg)
}

func TestConvertEventMetadataHostnameInTag(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	e, err := parseAndEnrichEventMessage(t, []byte("_e{10,9}:test title|test text|#host:localhost"), conf)

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, event.EventPriorityNormal, e.Priority)
	assert.Equal(t, "localhost", e.Host)
	assert.Equal(t, []string{}, e.Tags)
	assert.Equal(t, event.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
	assert.Equal(t, "", e.OriginInfo.FromUDS)
	assert.Equal(t, "", e.OriginInfo.FromTag)
	assert.Equal(t, "", e.OriginInfo.FromMsg)
}

func TestConvertEventMetadataEmptyHostTag(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	e, err := parseAndEnrichEventMessage(t, []byte("_e{10,9}:test title|test text|#host:,other:tag"), conf)

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, event.EventPriorityNormal, e.Priority)
	assert.Equal(t, "", e.Host)
	assert.Equal(t, []string{"other:tag"}, e.Tags)
	assert.Equal(t, event.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
	assert.Equal(t, "", e.OriginInfo.FromUDS)
	assert.Equal(t, "", e.OriginInfo.FromTag)
	assert.Equal(t, "", e.OriginInfo.FromMsg)
}

func TestConvertEventMetadataAlertType(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	e, err := parseAndEnrichEventMessage(t, []byte("_e{10,9}:test title|test text|t:warning"), conf)

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, event.EventPriorityNormal, e.Priority)
	assert.Equal(t, "default-hostname", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, event.EventAlertTypeWarning, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
	assert.Equal(t, "", e.OriginInfo.FromUDS)
	assert.Equal(t, "", e.OriginInfo.FromTag)
	assert.Equal(t, "", e.OriginInfo.FromMsg)
}

func TestConvertEventMetadataAggregatioKey(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	e, err := parseAndEnrichEventMessage(t, []byte("_e{10,9}:test title|test text|k:some aggregation key"), conf)

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, event.EventPriorityNormal, e.Priority)
	assert.Equal(t, "default-hostname", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, event.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "some aggregation key", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
	assert.Equal(t, "", e.OriginInfo.FromUDS)
	assert.Equal(t, "", e.OriginInfo.FromTag)
	assert.Equal(t, "", e.OriginInfo.FromMsg)
}

func TestConvertEventMetadataSourceType(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	e, err := parseAndEnrichEventMessage(t, []byte("_e{10,9}:test title|test text|s:this is the source"), conf)

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, event.EventPriorityNormal, e.Priority)
	assert.Equal(t, "default-hostname", e.Host)
	assert.Equal(t, []string(nil), e.Tags)
	assert.Equal(t, event.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "this is the source", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
	assert.Equal(t, "", e.OriginInfo.FromUDS)
	assert.Equal(t, "", e.OriginInfo.FromTag)
	assert.Equal(t, "", e.OriginInfo.FromMsg)
}

func TestConvertEventMetadataTags(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	e, err := parseAndEnrichEventMessage(t, []byte("_e{10,9}:test title|test text|#tag1,tag2:test"), conf)

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(0), e.Ts)
	assert.Equal(t, event.EventPriorityNormal, e.Priority)
	assert.Equal(t, "default-hostname", e.Host)
	assert.Equal(t, []string{"tag1", "tag2:test"}, e.Tags)
	assert.Equal(t, event.EventAlertTypeInfo, e.AlertType)
	assert.Equal(t, "", e.AggregationKey)
	assert.Equal(t, "", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
	assert.Equal(t, "", e.OriginInfo.FromUDS)
	assert.Equal(t, "", e.OriginInfo.FromTag)
	assert.Equal(t, "", e.OriginInfo.FromMsg)
}

func TestConvertEventMetadataMultiple(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	e, err := parseAndEnrichEventMessage(t, []byte("_e{10,9}:test title|test text|t:warning|d:12345|p:low|h:some.host|k:aggKey|s:source test|#tag1,tag2:test"), conf)

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(12345), e.Ts)
	assert.Equal(t, event.EventPriorityLow, e.Priority)
	assert.Equal(t, "some.host", e.Host)
	assert.Equal(t, []string{"tag1", "tag2:test"}, e.Tags)
	assert.Equal(t, event.EventAlertTypeWarning, e.AlertType)
	assert.Equal(t, "aggKey", e.AggregationKey)
	assert.Equal(t, "source test", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
	assert.Equal(t, "", e.OriginInfo.FromUDS)
	assert.Equal(t, "", e.OriginInfo.FromTag)
	assert.Equal(t, "", e.OriginInfo.FromMsg)
}

func TestEventOriginTag(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	e, err := parseAndEnrichEventMessage(t, []byte("_e{10,9}:test title|test text|t:warning|d:12345|p:low|h:some.host|k:aggKey|s:source test|#tag1,tag2:test,dd.internal.entity_id:testID"), conf)

	require.Nil(t, err)
	assert.Equal(t, "test title", e.Title)
	assert.Equal(t, "test text", e.Text)
	assert.Equal(t, int64(12345), e.Ts)
	assert.Equal(t, event.EventPriorityLow, e.Priority)
	assert.Equal(t, "some.host", e.Host)
	assert.Equal(t, []string{"tag1", "tag2:test"}, e.Tags)
	assert.Equal(t, event.EventAlertTypeWarning, e.AlertType)
	assert.Equal(t, "aggKey", e.AggregationKey)
	assert.Equal(t, "source test", e.SourceTypeName)
	assert.Equal(t, "", e.EventType)
	assert.Equal(t, "", e.OriginInfo.FromUDS)
	assert.Equal(t, "testID", e.OriginInfo.FromTag)
	assert.Equal(t, "", e.OriginInfo.FromMsg)

}
func TestConvertNamespace(t *testing.T) {
	conf := enrichConfig{
		metricPrefix:    "testNamespace.",
		defaultHostname: "default-hostname",
	}
	parsed, err := parseAndEnrichSingleMetricMessage(t, []byte("daemon:21|ms"), conf)

	assert.NoError(t, err)

	assert.Equal(t, "testNamespace.daemon", parsed.Name)
	assert.Equal(t, "default-hostname", parsed.Host)
}

func TestConvertNamespaceBlacklist(t *testing.T) {
	conf := enrichConfig{
		metricPrefix:          "testNamespace.",
		metricPrefixBlacklist: []string{"datadog.agent"},
		defaultHostname:       "default-hostname",
	}

	parsed, err := parseAndEnrichSingleMetricMessage(t, []byte("datadog.agent.daemon:21|ms"), conf)

	assert.NoError(t, err)

	assert.Equal(t, "datadog.agent.daemon", parsed.Name)
	assert.Equal(t, "default-hostname", parsed.Host)
}

func TestMetricBlocklistShouldBlock(t *testing.T) {

	message := []byte("custom.metric.a:21|ms")
	conf := enrichConfig{
		metricBlocklist: newBlocklist([]string{
			"custom.metric.a",
			"custom.metric.b",
		}, false),
		defaultHostname: "default",
	}

	deps := newServerDeps(t)
	parser := newParser(deps.Config, newFloat64ListPool(), 1, deps.WMeta)
	parsed, err := parser.parseMetricSample(message)
	assert.NoError(t, err)
	samples := []metrics.MetricSample{}
	samples = enrichMetricSample(samples, parsed, "", "", conf)

	assert.Equal(t, 0, len(samples))
}

func TestServerlessModeShouldSetEmptyHostname(t *testing.T) {
	conf := enrichConfig{
		serverlessMode:  true,
		defaultHostname: "default",
	}

	message := []byte("custom.metric.a:21|ms")
	deps := newServerDeps(t)
	parser := newParser(deps.Config, newFloat64ListPool(), 1, deps.WMeta)
	parsed, err := parser.parseMetricSample(message)
	assert.NoError(t, err)
	samples := []metrics.MetricSample{}
	samples = enrichMetricSample(samples, parsed, "", "", conf)

	assert.Equal(t, 1, len(samples))
	assert.Equal(t, "", samples[0].Host)
}

func TestMetricBlocklistShouldNotBlock(t *testing.T) {
	message := []byte("custom.metric.a:21|ms")
	conf := enrichConfig{
		metricBlocklist: newBlocklist([]string{
			"custom.metric.b",
			"custom.metric.c",
		}, false),
		defaultHostname: "default",
	}
	deps := newServerDeps(t)
	parser := newParser(deps.Config, newFloat64ListPool(), 1, deps.WMeta)
	parsed, err := parser.parseMetricSample(message)
	assert.NoError(t, err)
	samples := []metrics.MetricSample{}
	samples = enrichMetricSample(samples, parsed, "", "", conf)

	assert.Equal(t, 1, len(samples))
}

func TestConvertEntityOriginDetectionNoTags(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	parsed, err := parseAndEnrichSingleMetricMessage(t, []byte("daemon:666|g|#sometag1:somevalue1,host:my-hostname,dd.internal.entity_id:foo,sometag2:somevalue2"), conf)
	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, metrics.GaugeType, parsed.Mtype)
	require.Equal(t, 2, len(parsed.Tags))
	assert.Equal(t, "sometag1:somevalue1", parsed.Tags[0])
	assert.Equal(t, "sometag2:somevalue2", parsed.Tags[1])
	assert.Equal(t, "my-hostname", parsed.Host)
	assert.Equal(t, "", parsed.OriginInfo.FromUDS)
	assert.Equal(t, "foo", parsed.OriginInfo.FromTag)
	assert.Equal(t, "", parsed.OriginInfo.FromMsg)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertEntityOriginDetectionTags(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	parsed, err := parseAndEnrichSingleMetricMessage(t, []byte("daemon:666|g|#sometag1:somevalue1,host:my-hostname,dd.internal.entity_id:foo,sometag2:somevalue2"), conf)
	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, metrics.GaugeType, parsed.Mtype)
	require.Equal(t, 2, len(parsed.Tags))
	assert.ElementsMatch(t, []string{"sometag1:somevalue1", "sometag2:somevalue2"}, parsed.Tags)
	assert.Equal(t, "my-hostname", parsed.Host)
	assert.Equal(t, "", parsed.OriginInfo.FromUDS)
	assert.Equal(t, "foo", parsed.OriginInfo.FromTag)
	assert.Equal(t, "", parsed.OriginInfo.FromMsg)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestConvertEntityOriginDetectionTagsError(t *testing.T) {
	conf := enrichConfig{
		defaultHostname: "default-hostname",
	}
	parsed, err := parseAndEnrichSingleMetricMessage(t, []byte("daemon:666|g|#sometag1:somevalue1,host:my-hostname,dd.internal.entity_id:foo,sometag2:somevalue2"), conf)
	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.InEpsilon(t, 666.0, parsed.Value, epsilon)
	assert.Equal(t, metrics.GaugeType, parsed.Mtype)
	require.Equal(t, 2, len(parsed.Tags))
	assert.Equal(t, "sometag1:somevalue1", parsed.Tags[0])
	assert.Equal(t, "sometag2:somevalue2", parsed.Tags[1])
	assert.Equal(t, "my-hostname", parsed.Host)
	assert.Equal(t, "", parsed.OriginInfo.FromUDS)
	assert.Equal(t, "foo", parsed.OriginInfo.FromTag)
	assert.Equal(t, "", parsed.OriginInfo.FromMsg)
	assert.InEpsilon(t, 1.0, parsed.SampleRate, epsilon)
}

func TestEnrichTags(t *testing.T) {
	type args struct {
		tags          []string
		originFromUDS string
		originFromMsg []byte
		conf          enrichConfig
	}
	tests := []struct {
		name               string
		args               args
		wantedTags         []string
		wantedHost         string
		wantedOrigin       taggertypes.OriginInfo
		wantedMetricSource metrics.MetricSource
	}{
		{
			name: "empty tags, host=foo",
			args: args{
				originFromUDS: "",
				conf: enrichConfig{
					defaultHostname:           "foo",
					entityIDPrecedenceEnabled: true,
				},
			},
			wantedTags:         nil,
			wantedHost:         "foo",
			wantedOrigin:       taggertypes.OriginInfo{},
			wantedMetricSource: metrics.MetricSourceDogstatsd,
		},
		{
			name: "entityId not present, host=foo, should return origin tags",
			args: args{
				tags:          []string{"env:prod"},
				originFromUDS: "originID",
				conf: enrichConfig{
					defaultHostname:           "foo",
					entityIDPrecedenceEnabled: true,
				},
			},
			wantedTags:         []string{"env:prod"},
			wantedHost:         "foo",
			wantedOrigin:       taggertypes.OriginInfo{FromUDS: "originID"},
			wantedMetricSource: metrics.MetricSourceDogstatsd,
		},
		{
			name: "entityId not present, host=foo, empty tags list, should return origin tags",
			args: args{
				tags:          nil,
				originFromUDS: "originID",
				conf: enrichConfig{
					defaultHostname:           "foo",
					entityIDPrecedenceEnabled: true,
				},
			},
			wantedTags:         nil,
			wantedHost:         "foo",
			wantedOrigin:       taggertypes.OriginInfo{FromUDS: "originID"},
			wantedMetricSource: metrics.MetricSourceDogstatsd,
		},
		{
			name: "entityId present, host=foo, should not return origin tags",
			args: args{
				tags:          []string{"env:prod", fmt.Sprintf("%s%s", entityIDTagPrefix, "my-id")},
				originFromUDS: "originID",
				conf: enrichConfig{
					defaultHostname:           "foo",
					entityIDPrecedenceEnabled: true,
				},
			},
			wantedTags:         []string{"env:prod"},
			wantedHost:         "foo",
			wantedOrigin:       taggertypes.OriginInfo{FromUDS: "originID", FromTag: "my-id"},
			wantedMetricSource: metrics.MetricSourceDogstatsd,
		},
		{
			name: "entityId=none present, host=foo, should not call the originFromUDSFunc()",
			args: args{
				tags:          []string{"env:prod", fmt.Sprintf("%s%s", entityIDTagPrefix, "none")},
				originFromUDS: "originID",
				conf: enrichConfig{
					defaultHostname:           "foo",
					entityIDPrecedenceEnabled: true,
				},
			},
			wantedTags:         []string{"env:prod"},
			wantedHost:         "foo",
			wantedOrigin:       taggertypes.OriginInfo{FromUDS: "originID", FromTag: "none"},
			wantedMetricSource: metrics.MetricSourceDogstatsd,
		},
		{
			name: "entityId=42 present entityIDPrecendenceEnabled=false, host=foo, should call the originFromUDSFunc()",
			args: args{
				tags:          []string{"env:prod", fmt.Sprintf("%s%s", entityIDTagPrefix, "42")},
				originFromUDS: "originID",
				conf: enrichConfig{
					defaultHostname:           "foo",
					entityIDPrecedenceEnabled: false,
				},
			},
			wantedTags:         []string{"env:prod"},
			wantedHost:         "foo",
			wantedOrigin:       taggertypes.OriginInfo{FromUDS: "originID", FromTag: "42"},
			wantedMetricSource: metrics.MetricSourceDogstatsd,
		},
		{
			name: "entityId=42 cardinality=high present entityIDPrecendenceEnabled=false, host=foo, should call the originFromUDSFunc()",
			args: args{
				tags:          []string{"env:prod", fmt.Sprintf("%s%s", entityIDTagPrefix, "42"), CardinalityTagPrefix + collectors.HighCardinalityString},
				originFromUDS: "originID",
				conf: enrichConfig{
					defaultHostname:           "foo",
					entityIDPrecedenceEnabled: false,
				},
			},
			wantedTags:         []string{"env:prod"},
			wantedHost:         "foo",
			wantedOrigin:       taggertypes.OriginInfo{FromUDS: "originID", FromTag: "42", Cardinality: "high"},
			wantedMetricSource: metrics.MetricSourceDogstatsd,
		},
		{
			name: "entityId=42 cardinality=orchestrator present entityIDPrecendenceEnabled=false, host=foo, should call the originFromUDSFunc()",
			args: args{
				tags:          []string{"env:prod", fmt.Sprintf("%s%s", entityIDTagPrefix, "42"), CardinalityTagPrefix + collectors.OrchestratorCardinalityString},
				originFromUDS: "originID",
				conf: enrichConfig{
					defaultHostname:           "foo",
					entityIDPrecedenceEnabled: false,
				},
			},
			wantedTags:         []string{"env:prod"},
			wantedHost:         "foo",
			wantedOrigin:       taggertypes.OriginInfo{FromUDS: "originID", FromTag: "42", Cardinality: "orchestrator"},
			wantedMetricSource: metrics.MetricSourceDogstatsd,
		},
		{
			name: "entityId=42 cardinality=low present entityIDPrecendenceEnabled=false, host=foo, should call the originFromUDSFunc()",
			args: args{
				tags:          []string{"env:prod", fmt.Sprintf("%s%s", entityIDTagPrefix, "42"), CardinalityTagPrefix + collectors.LowCardinalityString},
				originFromUDS: "originID",
				conf: enrichConfig{
					defaultHostname:           "foo",
					entityIDPrecedenceEnabled: false,
				},
			},
			wantedTags:         []string{"env:prod"},
			wantedHost:         "foo",
			wantedOrigin:       taggertypes.OriginInfo{FromUDS: "originID", FromTag: "42", Cardinality: "low"},
			wantedMetricSource: metrics.MetricSourceDogstatsd,
		},
		{
			name: "entityId=42 cardinality=unknown present entityIDPrecendenceEnabled=false, host=foo, should call the originFromUDSFunc()",
			args: args{
				tags:          []string{"env:prod", fmt.Sprintf("%s%s", entityIDTagPrefix, "42"), CardinalityTagPrefix + collectors.UnknownCardinalityString},
				originFromUDS: "originID",
				conf: enrichConfig{
					defaultHostname:           "foo",
					entityIDPrecedenceEnabled: false,
				},
			},
			wantedTags:         []string{"env:prod"},
			wantedHost:         "foo",
			wantedOrigin:       taggertypes.OriginInfo{FromUDS: "originID", FromTag: "42", Cardinality: "unknown"},
			wantedMetricSource: metrics.MetricSourceDogstatsd,
		},
		{
			name: "entityId=42 cardinality='' present entityIDPrecendenceEnabled=false, host=foo, should call the originFromUDSFunc()",
			args: args{
				tags:          []string{"env:prod", fmt.Sprintf("%s%s", entityIDTagPrefix, "42"), CardinalityTagPrefix},
				originFromUDS: "originID",
				conf: enrichConfig{
					defaultHostname:           "foo",
					entityIDPrecedenceEnabled: false,
				},
			},
			wantedTags:         []string{"env:prod"},
			wantedHost:         "foo",
			wantedOrigin:       taggertypes.OriginInfo{FromUDS: "originID", FromTag: "42", Cardinality: ""},
			wantedMetricSource: metrics.MetricSourceDogstatsd,
		},
		{
			name: "entity_id=pod-uid, originFromMsg=container-id, should consider entity_id",
			args: args{
				tags:          []string{"env:prod", "dd.internal.entity_id:pod-uid"},
				originFromUDS: "originID",
				originFromMsg: []byte("container-id"),
				conf: enrichConfig{
					defaultHostname: "foo",
				},
			},
			wantedTags:         []string{"env:prod"},
			wantedHost:         "foo",
			wantedOrigin:       taggertypes.OriginInfo{FromUDS: "originID", FromTag: "pod-uid", FromMsg: "container-id"},
			wantedMetricSource: metrics.MetricSourceDogstatsd,
		},
		{
			name: "no entity_id, originFromMsg=container-id, should consider originFromMsg",
			args: args{
				tags:          []string{"env:prod"},
				originFromUDS: "originID",
				originFromMsg: []byte("container-id"),
				conf: enrichConfig{
					defaultHostname: "foo",
				},
			},
			wantedTags:         []string{"env:prod"},
			wantedHost:         "foo",
			wantedOrigin:       taggertypes.OriginInfo{FromUDS: "originID", FromMsg: "container-id"},
			wantedMetricSource: metrics.MetricSourceDogstatsd,
		},
	}
	for _, tt := range tests {
		tt.wantedOrigin.ProductOrigin = taggertypes.ProductOriginDogStatsD

		t.Run(tt.name, func(t *testing.T) {
			tags, host, origin, metricSource := extractTagsMetadata(tt.args.tags, tt.args.originFromUDS, tt.args.originFromMsg, tt.args.conf)
			assert.Equal(t, tt.wantedTags, tags)
			assert.Equal(t, tt.wantedHost, host)
			assert.Equal(t, tt.wantedOrigin, origin)
			assert.Equal(t, tt.wantedMetricSource, metricSource)
		})
	}
}

func TestEnrichTagsWithJMXCheckName(t *testing.T) {
	tests := []struct {
		name               string
		jmxCheckName       string
		tags               []string
		wantedTags         []string
		wantedMetricSource metrics.MetricSource
	}{
		{
			name:               "dd.internal.jmx_check_name:kafka, should give MetricSourceKafka",
			jmxCheckName:       "dd.internal.jmx_check_name:kafka",
			tags:               []string{"env:prod", "dd.internal.jmx_check_name:kafka"},
			wantedTags:         []string{"env:prod"},
			wantedMetricSource: metrics.MetricSourceKafka,
		},
		{
			name:               "dd.internal.jmx_check_name:cassandra, should give MetricSourceCassandra",
			jmxCheckName:       "dd.internal.jmx_check_name:cassandra",
			tags:               []string{"foo", "dd.internal.jmx_check_name:cassandra"},
			wantedTags:         []string{"foo"},
			wantedMetricSource: metrics.MetricSourceCassandra,
		},
		{
			name:               "dd.internal.jmx_check_name:tomcat, with jmx_domain tag should still set MetricSource",
			jmxCheckName:       "dd.internal.jmx_check_name:tomcat",
			tags:               []string{"foo", "jmx_domain:testdomain", "dd.internal.jmx_check_name:tomcat"},
			wantedTags:         []string{"foo", "jmx_domain:testdomain"},
			wantedMetricSource: metrics.MetricSourceTomcat,
		},
		{
			name:               "dd.internal.jmx_check_name:thisisacustomcheck, should give MetricSourceJmxCustom",
			jmxCheckName:       "dd.internal.jmx_check_name:thisisacustomcheck",
			tags:               []string{"env:prod", "dd.internal.jmx_check_name:thisisacustomcheck"},
			wantedTags:         []string{"env:prod"},
			wantedMetricSource: metrics.MetricSourceJmxCustom,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags, _, _, metricSource := extractTagsMetadata(tt.tags, "", []byte{}, enrichConfig{})
			assert.Equal(t, tt.wantedTags, tags)
			assert.Equal(t, tt.wantedMetricSource, metricSource)
			assert.NotContains(t, tags, tt.jmxCheckName)
		})

	}
}
