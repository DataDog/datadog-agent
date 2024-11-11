// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build test

package server

import (
	"net"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap/pidmapimpl"
	replaymock "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/fx-mock"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug/serverdebugimpl"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestHistToDist(t *testing.T) {
	cfg := make(map[string]interface{})

	cfg["dogstatsd_port"] = listeners.RandomPortName
	cfg["histogram_copy_to_distribution"] = true
	cfg["histogram_copy_to_distribution_prefix"] = "dist."

	deps := fulfillDepsWithConfigOverride(t, cfg)

	demux := deps.Demultiplexer
	requireStart(t, deps.Server)

	conn, err := net.Dial("udp", deps.Server.UDPLocalAddr())
	require.NoError(t, err, "cannot connect to DSD socket")
	defer conn.Close()

	// Test metric
	_, err = conn.Write([]byte("daemon:666|h|#sometag1:somevalue1,sometag2:somevalue2"))
	require.NoError(t, err, "cannot write to DSD socket")
	samples, timedSamples := demux.WaitForSamples(time.Second * 2)
	require.Equal(t, 2, len(samples))
	require.Equal(t, 0, len(timedSamples))
	histMetric := samples[0]
	distMetric := samples[1]
	assert.NotNil(t, histMetric)
	assert.Equal(t, histMetric.Name, "daemon")
	assert.EqualValues(t, histMetric.Value, 666.0)
	assert.Equal(t, metrics.HistogramType, histMetric.Mtype)

	assert.NotNil(t, distMetric)
	assert.Equal(t, distMetric.Name, "dist.daemon")
	assert.EqualValues(t, distMetric.Value, 666.0)
	assert.Equal(t, metrics.DistributionType, distMetric.Mtype)
	demux.Reset()
}

func TestExtraTags(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName
	cfg["dogstatsd_tags"] = []string{"sometag3:somevalue3"}

	env.SetFeatures(t, env.EKSFargate)
	deps := fulfillDepsWithConfigOverride(t, cfg)

	demux := deps.Demultiplexer
	requireStart(t, deps.Server)

	conn, err := net.Dial("udp", deps.Server.UDPLocalAddr())
	require.NoError(t, err, "cannot connect to DSD socket")
	defer conn.Close()

	// Test metric
	_, err = conn.Write([]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"))
	require.NoError(t, err, "cannot write to DSD socket")
	samples, timedSamples := demux.WaitForSamples(time.Second * 2)
	require.Equal(t, 1, len(samples))
	require.Equal(t, 0, len(timedSamples))
	sample := samples[0]
	assert.NotNil(t, sample)
	assert.Equal(t, sample.Name, "daemon")
	assert.EqualValues(t, sample.Value, 666.0)
	assert.Equal(t, sample.Mtype, metrics.GaugeType)
	assert.ElementsMatch(t, sample.Tags, []string{"sometag1:somevalue1", "sometag2:somevalue2", "sometag3:somevalue3"})
}

func TestParseMetricMessageTelemetry(t *testing.T) {
	cfg := make(map[string]interface{})

	cfg["dogstatsd_port"] = listeners.RandomPortName

	deps := fxutil.Test[depsWithoutServer](t, fx.Options(
		core.MockBundle(),
		serverdebugimpl.MockModule(),
		fx.Replace(configComponent.MockParams{
			Overrides: cfg,
		}),
		fx.Supply(Params{Serverless: false}),
		replaymock.MockModule(),
		compressionimpl.MockModule(),
		pidmapimpl.Module(),
		demultiplexerimpl.FakeSamplerMockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	s := newServerCompat(deps.Config, deps.Log, deps.Replay, deps.Debug, false, deps.Demultiplexer, deps.WMeta, deps.PidMap, deps.Telemetry)

	assert.Nil(t, s.mapper)

	var samples []metrics.MetricSample

	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)

	assert.Equal(t, float64(0), s.tlmProcessedOk.Get())
	samples, err := s.parseMetricMessage(samples, parser, []byte("test.metric:666|g"), "", "", false)
	assert.NoError(t, err)
	assert.Len(t, samples, 1)
	assert.Equal(t, float64(1), s.tlmProcessedOk.Get())

	assert.Equal(t, float64(0), s.tlmProcessedError.Get())
	samples, err = s.parseMetricMessage(samples, parser, nil, "", "", false)
	assert.Error(t, err, "invalid dogstatsd message format")
	assert.Len(t, samples, 1)
	assert.Equal(t, float64(1), s.tlmProcessedError.Get())
}

type MetricSample struct {
	Name  string
	Value float64
	Tags  []string
	Mtype metrics.MetricType
}

func TestMappingCases(t *testing.T) {
	scenarios := []struct {
		name              string
		config            string
		packets           []string
		expectedSamples   []MetricSample
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
			packets: []string{
				"test.job.duration.my_job_type.my_job_name:666|g",
				"test.job.size.my_job_type.my_job_name:666|g",
				"test.job.size.not_match:666|g",
			},
			expectedSamples: []MetricSample{
				{Name: "test.job.duration", Tags: []string{"job_type:my_job_type", "job_name:my_job_name"}, Mtype: metrics.GaugeType, Value: 666.0},
				{Name: "test.job.size", Tags: []string{"foo:my_job_type", "bar:my_job_name"}, Mtype: metrics.GaugeType, Value: 666.0},
				{Name: "test.job.size.not_match", Tags: nil, Mtype: metrics.GaugeType, Value: 666.0},
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
			packets: []string{
				"test.job.duration.my_job_type.my_job_name:666|g",
				"test.job.duration.my_job_type.my_job_name:666|g|#some:tag",
				"test.job.duration.my_job_type.my_job_name:666|g|#some:tag,more:tags",
			},
			expectedSamples: []MetricSample{
				{Name: "test.job.duration", Tags: []string{"job_type:my_job_type", "job_name:my_job_name"}, Mtype: metrics.GaugeType, Value: 666.0},
				{Name: "test.job.duration", Tags: []string{"job_type:my_job_type", "job_name:my_job_name", "some:tag"}, Mtype: metrics.GaugeType, Value: 666.0},
				{Name: "test.job.duration", Tags: []string{"job_type:my_job_type", "job_name:my_job_name", "some:tag", "more:tags"}, Mtype: metrics.GaugeType, Value: 666.0},
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
			packets:           []string{},
			expectedSamples:   nil,
			expectedCacheSize: 999,
		},
	}

	samples := []metrics.MetricSample{}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			deps := fulfillDepsWithConfigYaml(t, scenario.config)

			s := deps.Server.(*server)

			requireStart(t, s)

			assert.Equal(t, deps.Config.Get("dogstatsd_mapper_cache_size"), scenario.expectedCacheSize, "Case `%s` failed. cache_size `%s` should be `%s`", scenario.name, deps.Config.Get("dogstatsd_mapper_cache_size"), scenario.expectedCacheSize)

			var actualSamples []MetricSample
			for _, p := range scenario.packets {
				parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
				samples, err := s.parseMetricMessage(samples, parser, []byte(p), "", "", false)
				assert.NoError(t, err, "Case `%s` failed. parseMetricMessage should not return error %v", err)
				for _, sample := range samples {
					actualSamples = append(actualSamples, MetricSample{Name: sample.Name, Tags: sample.Tags, Mtype: sample.Mtype, Value: sample.Value})
				}
			}
			for _, sample := range scenario.expectedSamples {
				sort.Strings(sample.Tags)
			}
			for _, sample := range actualSamples {
				sort.Strings(sample.Tags)
			}
			assert.Equal(t, scenario.expectedSamples, actualSamples, "Case `%s` failed. `%s` should be `%s`", scenario.name, actualSamples, scenario.expectedSamples)
		})
	}
}

func TestParseEventMessageTelemetry(t *testing.T) {
	cfg := make(map[string]interface{})

	cfg["dogstatsd_port"] = listeners.RandomPortName

	deps := fxutil.Test[depsWithoutServer](t, fx.Options(
		core.MockBundle(),
		serverdebugimpl.MockModule(),
		fx.Replace(configComponent.MockParams{
			Overrides: cfg,
		}),
		fx.Supply(Params{Serverless: false}),
		replaymock.MockModule(),
		compressionimpl.MockModule(),
		pidmapimpl.Module(),
		demultiplexerimpl.FakeSamplerMockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	s := newServerCompat(deps.Config, deps.Log, deps.Replay, deps.Debug, false, deps.Demultiplexer, deps.WMeta, deps.PidMap, deps.Telemetry)

	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)

	telemetryMock, ok := deps.Telemetry.(telemetry.Mock)
	assert.True(t, ok)

	// three successful events
	s.parseEventMessage(parser, []byte("_e{10,10}:event title|test\\ntext|c:event-container"), "")
	s.parseEventMessage(parser, []byte("_e{10,10}:event title|test\\ntext|c:event-container"), "")
	s.parseEventMessage(parser, []byte("_e{10,10}:event title|test\\ntext|c:event-container"), "")
	// one error event
	_, err := s.parseEventMessage(parser, nil, "")
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

	deps := fxutil.Test[depsWithoutServer](t, fx.Options(
		core.MockBundle(),
		serverdebugimpl.MockModule(),
		fx.Replace(configComponent.MockParams{
			Overrides: cfg,
		}),
		fx.Supply(Params{Serverless: false}),
		replaymock.MockModule(),
		compressionimpl.MockModule(),
		pidmapimpl.Module(),
		demultiplexerimpl.FakeSamplerMockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	s := newServerCompat(deps.Config, deps.Log, deps.Replay, deps.Debug, false, deps.Demultiplexer, deps.WMeta, deps.PidMap, deps.Telemetry)

	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)

	telemetryMock, ok := deps.Telemetry.(telemetry.Mock)
	assert.True(t, ok)

	// three successful events
	s.parseServiceCheckMessage(parser, []byte("_sc|service-check.name|0|c:service-check-container"), "")
	s.parseServiceCheckMessage(parser, []byte("_sc|service-check.name|0|c:service-check-container"), "")
	s.parseServiceCheckMessage(parser, []byte("_sc|service-check.name|0|c:service-check-container"), "")
	// one error event
	_, err := s.parseServiceCheckMessage(parser, nil, "")
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
		samples, err := s.parseMetricMessage(samples, parser, []byte("test.metric:666|g"), "test_container", "1", false)
		assert.NoError(err)
		assert.Len(samples, 1)

		// one thing should have been stored when we parse a metric
		samples, err = s.parseMetricMessage(samples, parser, []byte("test.metric:555|g"), "test_container", "1", true)
		assert.NoError(err)
		assert.Len(samples, 2)
		assert.Len(s.cachedOriginCounters, 1, "one entry should have been cached")
		assert.Len(s.cachedOrder, 1, "one entry should have been cached")
		assert.Equal(s.cachedOrder[0].origin, "test_container")

		// when we parse another metric (different value) with same origin, cache should contain only one entry
		samples, err = s.parseMetricMessage(samples, parser, []byte("test.second_metric:525|g"), "test_container", "2", true)
		assert.NoError(err)
		assert.Len(samples, 3)
		assert.Len(s.cachedOriginCounters, 1, "one entry should have been cached")
		assert.Len(s.cachedOrder, 1, "one entry should have been cached")
		assert.Equal(s.cachedOrder[0].origin, "test_container")
		assert.Equal(s.cachedOrder[0].ok, map[string]string{"message_type": "metrics", "state": "ok", "origin": "test_container"})
		assert.Equal(s.cachedOrder[0].err, map[string]string{"message_type": "metrics", "state": "error", "origin": "test_container"})

		// when we parse another metric (different value) but with a different origin, we should store a new entry
		samples, err = s.parseMetricMessage(samples, parser, []byte("test.second_metric:525|g"), "another_container", "3", true)
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
		samples, err = s.parseMetricMessage(samples, parser, []byte("yetanothermetric:525|g"), "third_origin", "3", true)
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
		samples, err = s.parseMetricMessage(samples, parser, []byte("blablabla:555|g"), "fourth_origin", "4", true)
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
