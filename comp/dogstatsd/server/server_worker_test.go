// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build test

package server

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
)

// Run through all of the major metric types and verify both the default and the timestamped flows
func TestMetricTypes(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName
	deps := fulfillDepsWithConfigOverride(t, cfg)

	scenarios := []struct {
		name  string
		input []byte
		test  *tMetricSample
	}{
		{
			name:  "Test Gauge",
			input: []byte("daemon:666|g|@0.5|#sometag1:somevalue1,sometag2:somevalue2"),
			test:  defaultMetric().withType(metrics.GaugeType).withSampleRate(0.5),
		},
		{
			name:  "Test Counter",
			input: []byte("daemon:666|c|@0.5|#sometag1:somevalue1,sometag2:somevalue2"),
			test:  defaultMetric().withType(metrics.CounterType).withSampleRate(0.5),
		},
		{
			name:  "Test Histogram",
			input: []byte("daemon:666|h|@0.5|#sometag1:somevalue1,sometag2:somevalue2"),
			test:  defaultMetric().withType(metrics.HistogramType).withSampleRate(0.5),
		},
		{
			name:  "Test Timing",
			input: []byte("daemon:666|ms|@0.5|#sometag1:somevalue1,sometag2:somevalue2"),
			test:  defaultMetric().withType(metrics.HistogramType).withSampleRate(0.5)},
		{
			name:  "Test Set",
			input: []byte("daemon:abc|s|@0.5|#sometag1:somevalue1,sometag2:somevalue2"),
			test:  defaultMetric().withType(metrics.SetType).withSampleRate(0.5).withValue(0).withRawValue("abc"),
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			runTestMetrics(t, deps, s.input, []*tMetricSample{s.test}, []*tMetricSample{})

			timedInput := append(s.input, []byte("|T1658328888\n")...)
			s.test.withTimestamp(1658328888)
			runTestMetrics(t, deps, timedInput, []*tMetricSample{}, []*tMetricSample{s.test})
		})
	}
}

func TestMetricPermutations(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName
	deps := fulfillDepsWithConfigOverride(t, cfg)

	packet1Test := defaultMetric().withTags(nil).withType(metrics.CounterType)
	packet1AltTest := defaultMetric().withValue(123.0).withTags(nil).withType(metrics.CounterType)
	packet2Test := defaultMetric().withName("daemon2").withValue(1000.0).withType(metrics.CounterType)

	scenarios := []struct {
		name  string
		input []byte
		tests []*tMetricSample
	}{
		{
			name:  "Base multi-metric packet",
			input: []byte("daemon:666|c\ndaemon2:1000|c|#sometag1:somevalue1,sometag2:somevalue2"),
			tests: []*tMetricSample{packet1Test, packet2Test},
		},
		{
			name:  "Multi-value packet",
			input: []byte("daemon:666:123|c\ndaemon2:1000|c|#sometag1:somevalue1,sometag2:somevalue2"),
			tests: []*tMetricSample{packet1Test, packet1AltTest, packet2Test},
		},
		{
			name:  "Multi-value packet with skip empty",
			input: []byte("daemon::666::123::::|c\ndaemon2:1000|c|#sometag1:somevalue1,sometag2:somevalue2"),
			tests: []*tMetricSample{packet1Test, packet1AltTest, packet2Test},
		},
		{
			name:  "Malformed packet",
			input: []byte("daemon:666|c\n\ndaemon2:1000|c|#sometag1:somevalue1,sometag2:somevalue2\n"),
			tests: []*tMetricSample{packet1Test, packet2Test},
		},
		{
			name:  "Malformed metric",
			input: []byte("daemon:666a|g\ndaemon2:1000|c|#sometag1:somevalue1,sometag2:somevalue2"),
			tests: []*tMetricSample{packet2Test},
		},
		{
			name:  "Empty metric",
			input: []byte("daemon:|g\ndaemon2:1000|c|#sometag1:somevalue1,sometag2:somevalue2\ndaemon3: :1:|g"),
			tests: []*tMetricSample{packet2Test},
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			runTestMetrics(t, deps, s.input, s.tests, []*tMetricSample{})
		})
	}
}

func runTestMetrics(t *testing.T, deps serverDeps, input []byte, expTests []*tMetricSample, expTimeTests []*tMetricSample) {
	s := deps.Server.(*server)

	var b batcherMock
	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
	s.parsePackets(&b, parser, genTestPackets(input), metrics.MetricSampleBatch{}, nil)

	samples := b.samples
	timedSamples := b.lateSamples

	assert.Equal(t, len(expTests), len(samples))
	assert.Equal(t, len(expTimeTests), len(timedSamples))

	for idx, samp := range samples {
		expTests[idx].testMetric(t, samp)
	}
	for idx, tSamp := range timedSamples {
		expTimeTests[idx].testMetric(t, tSamp)
	}
}

func TestEvents(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName

	deps := fulfillDepsWithConfigOverride(t, cfg)
	s := deps.Server.(*server)
	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
	var b batcherMock

	input1 := defaultEventInput
	test1 := defaultEvent()
	input2 := []byte("_e{11,15}:test titled|test\\ntextntext|t:info|d:12346|p:normal|h:some.otherhost|k:aggKeyAlt|s:source investigation|#tag1,tag2:test,tag3:resolved")
	test2 := tEvent{
		Title:          "test titled",
		Text:           "test\ntextntext",
		Tags:           []string{"tag1", "tag2:test", "tag3:resolved"},
		Host:           "some.otherhost",
		Ts:             12346,
		AlertType:      event.AlertTypeInfo,
		Priority:       event.PriorityNormal,
		AggregationKey: "aggKeyAlt",
		SourceTypeName: "source investigation",
	}

	s.parsePackets(&b, parser, genTestPackets(input1, input2), metrics.MetricSampleBatch{}, nil)

	assert.Equal(t, 2, len(b.events))

	test1.testEvent(t, b.events[0])
	test2.testEvent(t, b.events[1])

	b.clear()
	// Test incomplete Events
	input := []byte("_e{0,9}:|test text\n" +
		string(defaultEventInput) + "\n" +
		"_e{-5,2}:abc\n",
	)

	s.parsePackets(&b, parser, genTestPackets(input), metrics.MetricSampleBatch{}, nil)
	assert.Equal(t, 1, len(b.events))
	defaultEvent().testEvent(t, b.events[0])
}

func TestServiceChecks(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName

	deps := fulfillDepsWithConfigOverride(t, cfg)
	s := deps.Server.(*server)
	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
	var b batcherMock

	s.parsePackets(&b, parser, genTestPackets(defaultServiceInput), metrics.MetricSampleBatch{}, nil)

	assert.Equal(t, 1, len(b.serviceChecks))
	defaultServiceCheck().testService(t, b.serviceChecks[0])

	b.clear()

	// Test incomplete Service Check
	input := append([]byte("_sc|agen.down\n"), defaultServiceInput...)
	s.parsePackets(&b, parser, genTestPackets(input), metrics.MetricSampleBatch{}, nil)

	assert.Equal(t, 1, len(b.serviceChecks))
	defaultServiceCheck().testService(t, b.serviceChecks[0])
}

func TestHistToDist(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName
	cfg["histogram_copy_to_distribution"] = true
	cfg["histogram_copy_to_distribution_prefix"] = "dist."
	deps := fulfillDepsWithConfigOverride(t, cfg)

	// Test metric
	input := []byte("daemon:666|h|#sometag1:somevalue1,sometag2:somevalue2")

	test1 := defaultMetric().withType(metrics.HistogramType)
	test2 := defaultMetric().withName("dist.daemon").withType(metrics.DistributionType)

	runTestMetrics(t, deps, input, []*tMetricSample{test1, test2}, []*tMetricSample{})
}

func TestExtraTags(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName

	deps := fulfillDepsWithConfigOverride(t, cfg)
	deps.Server.SetExtraTags([]string{"sometag3:somevalue3"})

	test := defaultMetric().withTags([]string{"sometag1:somevalue1", "sometag2:somevalue2", "sometag3:somevalue3"})
	// Test single metric
	runTestMetrics(t, deps, defaultMetricInput, []*tMetricSample{test}, []*tMetricSample{})

	// Test multivalue metric
	test2 := defaultMetric().withValue(500.0).withTags([]string{"sometag1:somevalue1", "sometag2:somevalue2", "sometag3:somevalue3"})
	input := []byte("daemon:666:500|g|#sometag1:somevalue1,sometag2:somevalue2")
	runTestMetrics(t, deps, input, []*tMetricSample{test, test2}, []*tMetricSample{})
}

func TestParseMetricMessageTelemetry(t *testing.T) {
	cfg := make(map[string]interface{})

	cfg["dogstatsd_port"] = listeners.RandomPortName

	deps, s := fulfillDepsWithInactiveServer(t, cfg)

	assert.Nil(t, s.mapper)

	var samples []metrics.MetricSample

	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)

	assert.Equal(t, float64(0), s.tlmProcessedOk.Get())
	samples, err := s.parseMetricMessage(samples, parser, []byte("test.metric:666|g"), "", 0, "", false, nil)
	assert.NoError(t, err)
	assert.Len(t, samples, 1)
	assert.Equal(t, float64(1), s.tlmProcessedOk.Get())

	assert.Equal(t, float64(0), s.tlmProcessedError.Get())
	samples, err = s.parseMetricMessage(samples, parser, nil, "", 0, "", false, nil)
	assert.Error(t, err, "invalid dogstatsd message format")
	assert.Len(t, samples, 1)
	assert.Equal(t, float64(1), s.tlmProcessedError.Get())
}

func TestMappingCases(t *testing.T) {
	scenarios := []struct {
		name              string
		config            string
		packets           [][]byte
		expectedSamples   []*tMetricSample
		expectedCacheSize int
	}{
		{
			name: "Simple OK case",
			config: `
dogstatsd_port: __random__
dogstatsd_mapper_profiles:
  - name: test
    prefix: 'test.'
    mappings:
      - match: "test.job.duration.*.*"
        name: "test.job.duration"
        tags:
          job_type: "$1"
          job_name: "$2"
      - match: "test.job.size.*.*"
        name: "test.job.size"
        tags:
          foo: "$1"
          bar: "$2"
`,
			packets: [][]byte{
				[]byte("test.job.duration.my_job_type.my_job_name:666|g"),
				[]byte("test.job.size.my_job_type.my_job_name:666|g"),
				[]byte("test.job.size.not_match:666|g"),
			},
			expectedSamples: []*tMetricSample{
				defaultMetric().withName("test.job.duration").withTags([]string{"job_type:my_job_type", "job_name:my_job_name"}),
				defaultMetric().withName("test.job.size").withTags([]string{"foo:my_job_type", "bar:my_job_name"}),
				defaultMetric().withName("test.job.size.not_match").withTags(nil),
			},
			expectedCacheSize: 1000,
		},
		{
			name: "Tag already present",
			config: `
dogstatsd_port: __random__
dogstatsd_mapper_profiles:
  - name: test
    prefix: 'test.'
    mappings:
      - match: "test.job.duration.*.*"
        name: "test.job.duration"
        tags:
          job_type: "$1"
          job_name: "$2"
`,
			packets: [][]byte{
				[]byte("test.job.duration.my_job_type.my_job_name:666|g"),
				[]byte("test.job.duration.my_job_type.my_job_name:666|g|#some:tag"),
				[]byte("test.job.duration.my_job_type.my_job_name:666|g|#some:tag,more:tags"),
			},
			expectedSamples: []*tMetricSample{
				defaultMetric().withName("test.job.duration").withTags([]string{"job_type:my_job_type", "job_name:my_job_name"}),
				defaultMetric().withName("test.job.duration").withTags([]string{"job_type:my_job_type", "job_name:my_job_name", "some:tag"}),
				defaultMetric().withName("test.job.duration").withTags([]string{"job_type:my_job_type", "job_name:my_job_name", "some:tag", "more:tags"}),
			},
			expectedCacheSize: 1000,
		},
		{
			name: "Cache size",
			config: `
dogstatsd_port: __random__
dogstatsd_mapper_cache_size: 999
dogstatsd_mapper_profiles:
  - name: test
    prefix: 'test.'
    mappings:
      - match: "test.job.duration.*.*"
        name: "test.job.duration"
        tags:
          job_type: "$1"
          job_name: "$2"
`,
			packets:           [][]byte{},
			expectedSamples:   nil,
			expectedCacheSize: 999,
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			deps := fulfillDepsWithConfigYaml(t, scenario.config)

			s := deps.Server.(*server)

			requireStart(t, s)

			assert.Equal(t, deps.Config.Get("dogstatsd_mapper_cache_size"), scenario.expectedCacheSize, "Case `%s` failed. cache_size `%s` should be `%s`", scenario.name, deps.Config.Get("dogstatsd_mapper_cache_size"), scenario.expectedCacheSize)

			parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
			var b batcherMock
			s.parsePackets(&b, parser, genTestPackets(scenario.packets...), metrics.MetricSampleBatch{}, nil)

			for idx, sample := range b.samples {
				scenario.expectedSamples[idx].testMetric(t, sample)
			}
		})
	}
}

func TestParseEventMessageTelemetry(t *testing.T) {
	cfg := make(map[string]interface{})

	cfg["dogstatsd_port"] = listeners.RandomPortName

	deps, s := fulfillDepsWithInactiveServer(t, cfg)

	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)

	telemetryMock, ok := deps.Telemetry.(telemetry.Mock)
	assert.True(t, ok)

	// three successful events
	s.parseEventMessage(parser, []byte("_e{10,10}:event title|test\\ntext|c:event-container"), "", 0)
	s.parseEventMessage(parser, []byte("_e{10,10}:event title|test\\ntext|c:event-container"), "", 0)
	s.parseEventMessage(parser, []byte("_e{10,10}:event title|test\\ntext|c:event-container"), "", 0)
	// one error event
	_, err := s.parseEventMessage(parser, nil, "", 0)
	assert.Error(t, err)

	processedEvents, err := telemetryMock.GetCountMetric("dogstatsd", "processed")
	require.NoError(t, err)

	for _, metric := range processedEvents {
		labels := metric.Tags()

		if labels["message_type"] == "events" && labels["state"] == "ok" {
			assert.Equal(t, float64(3), metric.Value())
		}

		if labels["message_type"] == "events" && labels["state"] == "error" {
			assert.Equal(t, float64(1), metric.Value())
		}
	}
}

func TestParseServiceCheckMessageTelemetry(t *testing.T) {
	cfg := make(map[string]interface{})

	cfg["dogstatsd_port"] = listeners.RandomPortName

	deps, s := fulfillDepsWithInactiveServer(t, cfg)

	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)

	telemetryMock, ok := deps.Telemetry.(telemetry.Mock)
	assert.True(t, ok)

	// three successful events
	s.parseServiceCheckMessage(parser, []byte("_sc|service-check.name|0|c:service-check-container"), "", 0)
	s.parseServiceCheckMessage(parser, []byte("_sc|service-check.name|0|c:service-check-container"), "", 0)
	s.parseServiceCheckMessage(parser, []byte("_sc|service-check.name|0|c:service-check-container"), "", 0)
	// one error event
	_, err := s.parseServiceCheckMessage(parser, nil, "", 0)
	assert.Error(t, err)

	processedEvents, err := telemetryMock.GetCountMetric("dogstatsd", "processed")
	require.NoError(t, err)

	for _, metric := range processedEvents {
		labels := metric.Tags()

		if labels["message_type"] == "service_checks" && labels["state"] == "ok" {
			assert.Equal(t, float64(3), metric.Value())
		}

		if labels["message_type"] == "service_checks" && labels["state"] == "error" {
			assert.Equal(t, float64(1), metric.Value())
		}
	}
}

func TestProcessedMetricsOrigin(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		cfg := make(map[string]interface{})
		cfg["dogstatsd_origin_optout_enabled"] = enabled
		cfg["dogstatsd_port"] = listeners.RandomPortName

		deps := fulfillDepsWithConfigOverride(t, cfg)
		s := deps.Server.(*server)
		assert := assert.New(t)

		s.Stop()

		assert.Len(s.cachedOriginCounters, 0, "this cache must be empty")
		assert.Len(s.cachedOrder, 0, "this cache list must be empty")

		parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
		samples := []metrics.MetricSample{}
		samples, err := s.parseMetricMessage(samples, parser, []byte("test.metric:666|g"), "test_container", 0, "1", false, nil)
		assert.NoError(err)
		assert.Len(samples, 1)

		// one thing should have been stored when we parse a metric
		samples, err = s.parseMetricMessage(samples, parser, []byte("test.metric:555|g"), "test_container", 0, "1", true, nil)
		assert.NoError(err)
		assert.Len(samples, 2)
		assert.Len(s.cachedOriginCounters, 1, "one entry should have been cached")
		assert.Len(s.cachedOrder, 1, "one entry should have been cached")
		assert.Equal(s.cachedOrder[0].origin, "test_container")

		// when we parse another metric (different value) with same origin, cache should contain only one entry
		samples, err = s.parseMetricMessage(samples, parser, []byte("test.second_metric:525|g"), "test_container", 0, "2", true, nil)
		assert.NoError(err)
		assert.Len(samples, 3)
		assert.Len(s.cachedOriginCounters, 1, "one entry should have been cached")
		assert.Len(s.cachedOrder, 1, "one entry should have been cached")
		assert.Equal(s.cachedOrder[0].origin, "test_container")
		assert.Equal(s.cachedOrder[0].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "test_container"})
		assert.Equal(s.cachedOrder[0].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "test_container"})

		// when we parse another metric (different value) but with a different origin, we should store a new entry
		samples, err = s.parseMetricMessage(samples, parser, []byte("test.second_metric:525|g"), "another_container", 0, "3", true, nil)
		assert.NoError(err)
		assert.Len(samples, 4)
		assert.Len(s.cachedOriginCounters, 2, "two entries should have been cached")
		assert.Len(s.cachedOrder, 2, "two entries should have been cached")
		assert.Equal(s.cachedOrder[0].origin, "test_container")
		assert.Equal(s.cachedOrder[0].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "test_container"})
		assert.Equal(s.cachedOrder[0].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "test_container"})
		assert.Equal(s.cachedOrder[1].origin, "another_container")
		assert.Equal(s.cachedOrder[1].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "another_container"})
		assert.Equal(s.cachedOrder[1].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "another_container"})

		// oldest one should be removed once we reach the limit of the cache
		maxOriginCounters = 2
		samples, err = s.parseMetricMessage(samples, parser, []byte("yetanothermetric:525|g"), "third_origin", 0, "3", true, nil)
		assert.NoError(err)
		assert.Len(samples, 5)
		assert.Len(s.cachedOriginCounters, 2, "two entries should have been cached, one has been evicted already")
		assert.Len(s.cachedOrder, 2, "two entries should have been cached, one has been evicted already")
		assert.Equal(s.cachedOrder[0].origin, "another_container")
		assert.Equal(s.cachedOrder[0].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "another_container"})
		assert.Equal(s.cachedOrder[0].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "another_container"})
		assert.Equal(s.cachedOrder[1].origin, "third_origin")
		assert.Equal(s.cachedOrder[1].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "third_origin"})
		assert.Equal(s.cachedOrder[1].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "third_origin"})

		// oldest one should be removed once we reach the limit of the cache
		maxOriginCounters = 2
		samples, err = s.parseMetricMessage(samples, parser, []byte("blablabla:555|g"), "fourth_origin", 0, "4", true, nil)
		assert.NoError(err)
		assert.Len(samples, 6)
		assert.Len(s.cachedOriginCounters, 2, "two entries should have been cached, two have been evicted already")
		assert.Len(s.cachedOrder, 2, "two entries should have been cached, two have been evicted already")
		assert.Equal(s.cachedOrder[0].origin, "third_origin")
		assert.Equal(s.cachedOrder[0].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "third_origin"})
		assert.Equal(s.cachedOrder[0].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "third_origin"})
		assert.Equal(s.cachedOrder[1].origin, "fourth_origin")
		assert.Equal(s.cachedOrder[1].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "fourth_origin"})
		assert.Equal(s.cachedOrder[1].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "fourth_origin"})
	}
}

func TestNextMessage(t *testing.T) {
	scenarios := []struct {
		name              string
		messages          []string
		eolTermination    bool
		expectedTlm       int64
		expectedMetritCnt int
	}{
		{
			name:              "No eol newline, eol enabled",
			messages:          []string{"foo\n", "bar\r\n", "baz\r\n", "quz\n", "hax\r"},
			eolTermination:    true,
			expectedTlm:       1,
			expectedMetritCnt: 4, // final message won't be processed, no newline
		},
		{
			name:              "No eol newline, eol disabled",
			messages:          []string{"foo\n", "bar\r\n", "baz\r\n", "quz\n", "hax"},
			eolTermination:    false,
			expectedTlm:       0,
			expectedMetritCnt: 5,
		},
		{
			name:              "Base Case",
			messages:          []string{"foo\n", "bar\r\n", "baz\r\n", "quz\n", "hax\r\n"},
			eolTermination:    true,
			expectedTlm:       0,
			expectedMetritCnt: 5,
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			packet := []byte(strings.Join(s.messages, ""))
			initialTelem := dogstatsdUnterminatedMetricErrors.Value()
			res := nextMessage(&packet, s.eolTermination)
			cnt := 0
			for res != nil {
				// Confirm newline/carriage return were not transferred
				assert.Equal(t, string(res), strings.TrimRight(s.messages[cnt], "\n\r"))
				res = nextMessage(&packet, s.eolTermination)
				cnt++
			}

			assert.Equal(t, s.expectedTlm, dogstatsdUnterminatedMetricErrors.Value()-initialTelem)
			assert.Equal(t, s.expectedMetritCnt, cnt)
		})
	}
}
