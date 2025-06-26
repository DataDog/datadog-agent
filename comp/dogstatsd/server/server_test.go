// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package server

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/mapper"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestNewServer(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName

	deps := fulfillDepsWithConfigOverride(t, cfg)
	requireStart(t, deps.Server)

}

func TestHistogramMetricNamesFilter(t *testing.T) {
	cfg := make(map[string]interface{})
	require := require.New(t)

	cfg["histogram_aggregates"] = []string{"avg", "max", "median"}
	cfg["histogram_percentiles"] = []string{"0.73", "0.22"}

	deps := fulfillDepsWithConfigOverride(t, cfg)
	s := deps.Server.(*server)

	bl := []string{
		"foo",
		"bar",
		"baz",
		"foomax",
		"foo.avg",
		"foo.max",
		"foo.count",
		"baz.73percentile",
		"bar.50percentile",
		"bar.22percentile",
		"count",
	}

	filtered := s.createHistogramsBlocklist(bl)
	require.ElementsMatch(filtered, []string{"foo.avg", "foo.max", "baz.73percentile", "bar.22percentile"})
}

// This test is proving that no data race occurred on the `cachedTlmOriginIds` map.
// It should not fail since `cachedTlmOriginIds` and `cachedOrder` should be
// properly protected from multiple accesses by `cachedTlmLock`.
// The main purpose of this test is to detect early if a future code change is
// introducing a data race.
func TestNoRaceOriginTagMaps(t *testing.T) {
	const N = 100
	cfg := make(map[string]interface{})

	cfg["dogstatsd_port"] = listeners.RandomPortName

	_, s := fulfillDepsWithInactiveServer(t, cfg)

	sync := make(chan struct{})
	done := make(chan struct{}, N)
	for i := 0; i < N; i++ {
		id := fmt.Sprintf("%d", i)
		go func() {
			defer func() { done <- struct{}{} }()
			<-sync
			s.getOriginCounter(id)
		}()
	}
	close(sync)
	for i := 0; i < N; i++ {
		<-done
	}
}

func TestNoMappingsConfig(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName
	deps := fulfillDepsWithConfigOverride(t, cfg)
	s := deps.Server.(*server)
	cw := deps.Config.(model.Writer)
	cw.SetWithoutSource("dogstatsd_port", listeners.RandomPortName)

	samples := []metrics.MetricSample{}

	requireStart(t, s)

	assert.Nil(t, s.mapper)

	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
	samples, err := s.parseMetricMessage(samples, parser, []byte("test.metric:666|g"), "", 0, "", false, nil)
	assert.NoError(t, err)
	assert.Len(t, samples, 1)
}

func TestNewServerExtraTags(t *testing.T) {
	cfg := make(map[string]interface{})

	require := require.New(t)
	cfg["dogstatsd_port"] = listeners.RandomPortName

	deps := fulfillDepsWithConfigOverride(t, cfg)
	s := deps.Server.(*server)
	requireStart(t, s)
	require.Len(s.extraTags, 0, "no tags should have been read")

	// when not running in fargate, the tags entry is not used
	cfg["tags"] = "hello:world"
	deps = fulfillDepsWithConfigOverride(t, cfg)
	s = deps.Server.(*server)
	requireStart(t, s)
	require.Len(s.extraTags, 0, "no tags should have been read")

	// dogstatsd_tag is always pulled in to extra tags
	cfg["dogstatsd_tags"] = "hello:world2 extra:tags"
	deps = fulfillDepsWithConfigOverride(t, cfg)
	s = deps.Server.(*server)
	requireStart(t, s)
	require.ElementsMatch([]string{"extra:tags", "hello:world2"}, s.extraTags, "two tags should have been read")
	require.Len(s.extraTags, 2, "two tags should have been read")
	require.Equal(s.extraTags[0], "extra:tags", "the tag extra:tags should be set")
	require.Equal(s.extraTags[1], "hello:world2", "the tag hello:world should be set")

	// when running in fargate, "tags" and "dogstatsd_tag" configs are conjoined
	env.SetFeatures(t, env.EKSFargate)
	deps = fulfillDepsWithConfigOverride(t, cfg)
	s = deps.Server.(*server)
	requireStart(t, s)

	require.ElementsMatch(
		[]string{"hello:world", "extra:tags", "hello:world2"},
		s.extraTags,
		"both tag sources should have been combined",
	)
}

//nolint:revive // TODO(AML) Fix revive linter
func testContainerIDParsing(t *testing.T, cfg map[string]interface{}) {
	cfg["dogstatsd_port"] = listeners.RandomPortName
	deps := fulfillDepsWithConfigOverride(t, cfg)
	s := deps.Server.(*server)
	assert := assert.New(t)
	requireStart(t, s)

	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
	parser.dsdOriginEnabled = true

	// Metric
	metrics, err := s.parseMetricMessage(nil, parser, []byte("metric.name:123|g|c:metric-container"), "", 0, "", false, nil)
	assert.NoError(err)
	assert.Len(metrics, 1)
	assert.Equal("metric-container", metrics[0].OriginInfo.LocalData.ContainerID)

	// Event
	event, err := s.parseEventMessage(parser, []byte("_e{10,10}:event title|test\\ntext|c:event-container"), "", 0)
	assert.NoError(err)
	assert.NotNil(event)
	assert.Equal("event-container", event.OriginInfo.LocalData.ContainerID)

	// Service check
	serviceCheck, err := s.parseServiceCheckMessage(parser, []byte("_sc|service-check.name|0|c:service-check-container"), "", 0)
	assert.NoError(err)
	assert.NotNil(serviceCheck)
	assert.Equal("service-check-container", serviceCheck.OriginInfo.LocalData.ContainerID)
}

func TestContainerIDParsing(t *testing.T) {
	cfg := make(map[string]interface{})

	for _, enabled := range []bool{true, false} {

		cfg["dogstatsd_origin_optout_enabled"] = enabled
		t.Run(fmt.Sprintf("optout_enabled=%v", enabled), func(t *testing.T) {
			testContainerIDParsing(t, cfg)
		})
	}
}

func TestOrigin(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName
	t.Run("TestOrigin", func(t *testing.T) {
		deps := fulfillDepsWithConfigOverride(t, cfg)
		s := deps.Server.(*server)
		assert := assert.New(t)

		requireStart(t, s)

		parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
		parser.dsdOriginEnabled = true

		// Metric
		metrics, err := s.parseMetricMessage(nil, parser, []byte("metric.name:123|g|c:metric-container|#dd.internal.card:none"), "", 1234, "", false, nil)
		assert.NoError(err)
		assert.Len(metrics, 1)
		assert.Equal("metric-container", metrics[0].OriginInfo.LocalData.ContainerID)
		assert.Equal(uint32(1234), metrics[0].OriginInfo.LocalData.ProcessID)

		// Event
		event, err := s.parseEventMessage(parser, []byte("_e{10,10}:event title|test\\ntext|c:event-container|#dd.internal.card:none"), "", 1234)
		assert.NoError(err)
		assert.NotNil(event)
		assert.Equal("event-container", event.OriginInfo.LocalData.ContainerID)
		assert.Equal(uint32(1234), event.OriginInfo.LocalData.ProcessID)

		// Service check
		serviceCheck, err := s.parseServiceCheckMessage(parser, []byte("_sc|service-check.name|0|c:service-check-container|#dd.internal.card:none"), "", 1234)
		assert.NoError(err)
		assert.NotNil(serviceCheck)
		assert.Equal("service-check-container", serviceCheck.OriginInfo.LocalData.ContainerID)
		assert.Equal(uint32(1234), serviceCheck.OriginInfo.LocalData.ProcessID)
	})
}

func requireStart(t *testing.T, s Component) {
	assert.NotNil(t, s)
	assert.True(t, s.IsRunning(), "server was not running")
}

func TestDogstatsdMappingProfilesOk(t *testing.T) {
	datadogYaml := `
dogstatsd_mapper_profiles:
  - name: "airflow"
    prefix: "airflow."
    mappings:
      - match: 'airflow\.job\.duration_sec\.(.*)'
        name: "airflow.job.duration"
        match_type: "regex"
        tags:
          job_type: "$1"
          job_name: "$2"
      - match: "airflow.job.size.*.*"
        name: "airflow.job.size"
        tags:
          foo: "$1"
          bar: "$2"
  - name: "profile2"
    prefix: "profile2."
    mappings:
      - match: "profile2.hello.*"
        name: "profile2.hello"
        tags:
          foo: "$1"
`
	testConfig := configmock.NewFromYAML(t, datadogYaml)

	profiles, err := getDogstatsdMappingProfiles(testConfig)
	require.NoError(t, err)

	expectedProfiles := []mapper.MappingProfileConfig{
		{
			Name:   "airflow",
			Prefix: "airflow.",
			Mappings: []mapper.MetricMappingConfig{
				{
					Match:     "airflow\\.job\\.duration_sec\\.(.*)",
					MatchType: "regex",
					Name:      "airflow.job.duration",
					Tags:      map[string]string{"job_type": "$1", "job_name": "$2"},
				},
				{
					Match: "airflow.job.size.*.*",
					Name:  "airflow.job.size",
					Tags:  map[string]string{"foo": "$1", "bar": "$2"},
				},
			},
		},
		{
			Name:   "profile2",
			Prefix: "profile2.",
			Mappings: []mapper.MetricMappingConfig{
				{
					Match: "profile2.hello.*",
					Name:  "profile2.hello",
					Tags:  map[string]string{"foo": "$1"},
				},
			},
		},
	}
	assert.EqualValues(t, expectedProfiles, profiles)
}

func TestDogstatsdMappingProfilesEmpty(t *testing.T) {
	datadogYaml := `
dogstatsd_mapper_profiles:
`
	testConfig := configmock.NewFromYAML(t, datadogYaml)

	profiles, err := getDogstatsdMappingProfiles(testConfig)

	var expectedProfiles []mapper.MappingProfileConfig

	assert.NoError(t, err)
	assert.EqualValues(t, expectedProfiles, profiles)
}

func TestDogstatsdMappingProfilesError(t *testing.T) {
	datadogYaml := `
dogstatsd_mapper_profiles:
  - abc
`
	testConfig := configmock.NewFromYAML(t, datadogYaml)

	profiles, err := getDogstatsdMappingProfiles(testConfig)

	expectedErrorMsg := "Could not parse dogstatsd_mapper_profiles"
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), expectedErrorMsg)
	assert.Empty(t, profiles)
}

func TestDogstatsdMappingProfilesEnv(t *testing.T) {
	env := "DD_DOGSTATSD_MAPPER_PROFILES"
	t.Setenv(env, `[
{"name":"another_profile","prefix":"abcd","mappings":[
	{
		"match":"airflow\\.dag_processing\\.last_runtime\\.(.*)",
		"match_type":"regex","name":"foo",
		"tags":{"a":"$1","b":"$2"}
	}]},
{"name":"some_other_profile","prefix":"some_other_profile.","mappings":[{"match":"some_other_profile.*","name":"some_other_profile.abc","tags":{"a":"$1"}}]}
]`)
	expected := []mapper.MappingProfileConfig{
		{Name: "another_profile", Prefix: "abcd", Mappings: []mapper.MetricMappingConfig{
			{Match: "airflow\\.dag_processing\\.last_runtime\\.(.*)", MatchType: "regex", Name: "foo", Tags: map[string]string{"a": "$1", "b": "$2"}},
		}},
		{Name: "some_other_profile", Prefix: "some_other_profile.", Mappings: []mapper.MetricMappingConfig{
			{Match: "some_other_profile.*", Name: "some_other_profile.abc", Tags: map[string]string{"a": "$1"}},
		}},
	}
	cfg := configmock.New(t)
	mappings, _ := getDogstatsdMappingProfiles(cfg)
	assert.Equal(t, expected, mappings)
}
