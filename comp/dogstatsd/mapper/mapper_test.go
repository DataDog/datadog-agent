// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test
// +build test

package mapper

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestMappings(t *testing.T) {
	scenarios := []struct {
		name            string
		config          string
		packets         []string
		expectedResults []MapResult
	}{
		{
			name: "Simple OK case",
			config: `
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
				"test.job.duration.my_job_type.my_job_name",
				"test.job.size.my_job_type.my_job_name",
				"test.job.size.not_match",
			},
			expectedResults: []MapResult{
				{Name: "test.job.duration", Tags: []string{"job_type:my_job_type", "job_name:my_job_name"}, matched: true},
				{Name: "test.job.size", Tags: []string{"foo:my_job_type", "bar:my_job_name"}, matched: true},
			},
		},
		{
			name: "Not/Partially mapped are accepted",
			config: `
dogstatsd_mapper_profiles:
  - name: test
    prefix: 'test.'
    mappings:
      - match: "test.job.duration.*.*"
        name: "test.job.duration"
        tags:
          job_type: "$1"
      - match: "test.task.duration.*.*"
        name: "test.task.duration"
`,
			packets: []string{
				"test.job.duration.my_job_type.my_job_name",
				"test.task.duration.my_job_type.my_job_name",
			},
			expectedResults: []MapResult{
				{Name: "test.job.duration", Tags: []string{"job_type:my_job_type"}, matched: true},
				{Name: "test.task.duration", Tags: make([]string, 0), matched: true},
			},
		},
		{
			name: "Use regex expansion alternative syntax",
			config: `
dogstatsd_mapper_profiles:
 - name: test
   prefix: 'test.'
   mappings:
    - match: "test.job.duration.*.*"
      name: "test.job.duration"
      tags:
        job_type: "${1}_x"
        job_name: "${2}_y"
`,
			packets: []string{
				"test.job.duration.my_job_type.my_job_name",
			},
			expectedResults: []MapResult{
				{Name: "test.job.duration", Tags: []string{"job_type:my_job_type_x", "job_name:my_job_name_y"}, matched: true},
			},
		},
		{
			name: "Expand name",
			config: `
dogstatsd_mapper_profiles:
 - name: test
   prefix: 'test.'
   mappings:
    - match: "test.job.duration.*.*"
      name: "test.hello.$2.$1"
      tags:
        job_type: "$1"
        job_name: "$2"
`,
			packets: []string{
				"test.job.duration.my_job_type.my_job_name",
			},
			expectedResults: []MapResult{
				{Name: "test.hello.my_job_name.my_job_type", Tags: []string{"job_type:my_job_type", "job_name:my_job_name"}, matched: true},
			},
		},
		{
			name: "Match before underscore",
			config: `
dogstatsd_mapper_profiles:
 - name: test
   prefix: 'test.'
   mappings:
    - match: "test.*_start"
      name: "test.start"
      tags:
        job: "$1"
`,
			packets: []string{
				"test.my_job_start",
			},
			expectedResults: []MapResult{
				{Name: "test.start", Tags: []string{"job:my_job"}, matched: true},
			},
		},
		{
			name: "No tags",
			config: `
dogstatsd_mapper_profiles:
 - name: test
   prefix: 'test.'
   mappings:
    - match: "test.my-worker.start"
      name: "test.worker.start"
    - match: "test.my-worker.stop.*"
      name: "test.worker.stop"
`,
			packets: []string{
				"test.my-worker.start",
				"test.my-worker.stop.worker-name",
			},
			expectedResults: []MapResult{
				{Name: "test.worker.start", Tags: make([]string, 0), matched: true},
				{Name: "test.worker.stop", Tags: make([]string, 0), matched: true},
			},
		},
		{
			name: "All allowed characters",
			config: `
dogstatsd_mapper_profiles:
 - name: test
   prefix: 'test.'
   mappings:
    - match: "test.abcdefghijklmnopqrstuvwxyz_ABCDEFGHIJKLMNOPQRSTUVWXYZ-01234567.*"
      name: "test.alphabet"
`,
			packets: []string{
				"test.abcdefghijklmnopqrstuvwxyz_ABCDEFGHIJKLMNOPQRSTUVWXYZ-01234567.123",
			},
			expectedResults: []MapResult{
				{Name: "test.alphabet", Tags: make([]string, 0), matched: true},
			},
		},
		{
			name: "Regex match type",
			config: `
dogstatsd_mapper_profiles:
 - name: test
   prefix: 'test.'
   mappings:
    - match: "test\\.job\\.duration\\.(.*)"
      match_type: regex
      name: "test.job.duration"
      tags:
        job_name: "$1"
    - match: 'test\.task\.duration\.(.*)' # no need to escape using single quote
      match_type: regex
      name: "test.task.duration"
      tags:
        task_name: "$1"
`,
			packets: []string{
				"test.job.duration.my.funky.job$name-abc/123",
				"test.task.duration.MY_task_name",
			},
			expectedResults: []MapResult{
				{Name: "test.job.duration", Tags: []string{"job_name:my.funky.job$name-abc/123"}, matched: true},
				{Name: "test.task.duration", Tags: []string{"task_name:MY_task_name"}, matched: true},
			},
		},
		{
			name: "Complex Regex match type",
			config: `
dogstatsd_mapper_profiles:
  - name: test
    prefix: 'test.'
    mappings:
      - match: "test\\.job\\.([a-z][0-9]-\\w+)\\.(.*)"
        match_type: regex
        name: "test.job"
        tags:
          job_type: "$1"
          job_name: "$2"
`,
			packets: []string{
				"test.job.a5-foo.bar",
				"test.job.foo.bar-not-match",
			},
			expectedResults: []MapResult{
				{Name: "test.job", Tags: []string{"job_type:a5-foo", "job_name:bar"}, matched: true},
			},
		},
		{
			name: "Profile and prefix",
			config: `
dogstatsd_mapper_profiles:
  - name: test
    prefix: 'foo.'
    mappings:
      - match: "foo.duration.*"
        name: "foo.duration"
        tags:
          name: "$1"
  - name: test
    prefix: 'bar.'
    mappings:
      - match: "bar.count.*"
        name: "bar.count"
        tags:
          name: "$1"
      - match: "foo.duration2.*"
        name: "foo.duration2"
        tags:
          name: "$1"
`,
			packets: []string{
				"foo.duration.foo_name1",
				"foo.duration2.foo_name1", // should not match, metric in wrong group.
				"bar.count.bar_name1",
				"z.not.mapped",
			},
			expectedResults: []MapResult{
				{Name: "foo.duration", Tags: []string{"name:foo_name1"}, matched: true},
				{Name: "bar.count", Tags: []string{"name:bar_name1"}, matched: true},
			},
		},
		{
			name: "Wildcard prefix",
			config: `
dogstatsd_mapper_profiles:
  - name: test
    prefix: '*'
    mappings:
      - match: "foo.duration.*"
        name: "foo.duration"
        tags:
          name: "$1"
`,
			packets: []string{
				"foo.duration.foo_name1",
			},
			expectedResults: []MapResult{
				{Name: "foo.duration", Tags: []string{"name:foo_name1"}, matched: true},
			},
		},
		{
			name: "Wildcard prefix order",
			config: `
dogstatsd_mapper_profiles:
  - name: test
    prefix: '*'
    mappings:
      - match: "foo.duration.*"
        name: "foo.duration"
        tags:
          name1: "$1"
  - name: test
    prefix: '*'
    mappings:
      - match: "foo.duration.*"
        name: "foo.duration"
        tags:
          name2: "$1"
`,
			packets: []string{
				"foo.duration.foo_name",
			},
			expectedResults: []MapResult{
				{Name: "foo.duration", Tags: []string{"name1:foo_name"}, matched: true},
			},
		},
		{
			name: "Multiple profiles order",
			config: `
dogstatsd_mapper_profiles:
  - name: test
    prefix: 'foo.'
    mappings:
      - match: "foo.*.duration.*"
        name: "foo.bar1.duration"
        tags:
          bar: "$1"
          foo: "$2"
  - name: test
    prefix: 'foo.bar.'
    mappings:
      - match: "foo.bar.duration.*"
        name: "foo.bar2.duration"
        tags:
          foo_bar: "$1"
`,
			packets: []string{
				"foo.bar.duration.foo_name",
			},
			expectedResults: []MapResult{
				{Name: "foo.bar1.duration", Tags: []string{"bar:bar", "foo:foo_name"}, matched: true},
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			mapper, err := getMapper(t, scenario.config)
			require.NoError(t, err)

			var actualResults []MapResult
			for _, packet := range scenario.packets {
				mapResult := mapper.Map(packet)
				if mapResult != nil {
					actualResults = append(actualResults, *mapResult)
				}
			}
			for _, sample := range scenario.expectedResults {
				sort.Strings(sample.Tags)
			}
			for _, sample := range actualResults {
				sort.Strings(sample.Tags)
			}
			assert.Equal(t, scenario.expectedResults, actualResults, "Case `%s` failed. `%s` should be `%s`", scenario.name, actualResults, scenario.expectedResults)
		})
	}
}

func TestMappingErrors(t *testing.T) {
	scenarios := []struct {
		name          string
		config        string
		packets       []string
		expectedError string
	}{
		{
			name: "Empty name",
			config: `
dogstatsd_mapper_profiles:
  - name: test
    prefix: 'test.'
    mappings:
      - match: "test.job.duration.*.*"
        name: ""
        tags:
          job_type: "$1"
          job_name: "$2"
`,
			packets: []string{
				"test.job.duration.my_job_type.my_job_name",
			},
			expectedError: "name is required",
		},
		{
			name: "Missing name",
			config: `
dogstatsd_mapper_profiles:
  - name: test
    prefix: 'test.'
    mappings:
      - match: "test.job.duration.*.*"
        tags:
          job_type: "$1"
          job_name: "$2"
`,
			packets: []string{
				"test.job.duration.my_job_type.my_job_name",
			},
			expectedError: "name is required",
		},
		{
			name: "Missing match",
			config: `
dogstatsd_mapper_profiles:
  - name: test
    prefix: 'test.'
    mappings:
      - name: "test.job.duration"
        tags:
          job_type: "$1"
          job_name: "$2"
`,
			packets: []string{
				"test.job.duration.my_job_type.my_job_name",
			},
			expectedError: "match is required",
		},
		{
			name: "Invalid match regex []",
			config: `
dogstatsd_mapper_profiles:
  - name: test
    prefix: 'test.'
    mappings:
      - match: "test.[]duration.*.*"
        name: "test.job.duration"
`,
			packets: []string{
				"test.job.duration.my_job_type.my_job_name",
			},
			expectedError: "it does not match allowed match regex",
		},
		{
			name: "Invalid match regex ^",
			config: `
dogstatsd_mapper_profiles:
  - name: test
    prefix: 'test.'
    mappings:
      - match: "^test.invalid.duration.*.*"
        name: "test.job.duration"
`,
			packets: []string{
				"test.job.duration.my_job_type.my_job_name",
			},
			expectedError: "it does not match allowed match regex",
		},
		{
			name: "Consecutive *",
			config: `
dogstatsd_mapper_profiles:
  - name: test
    prefix: 'test.'
    mappings:
      - match: "test.invalid.duration.**"
        name: "test.job.duration"
`,
			packets: []string{
				"test.job.duration.my_job_type.my_job_name",
			},
			expectedError: "it should not contain consecutive `*`",
		},
		{
			name: "Invalid match type",
			config: `
dogstatsd_mapper_profiles:
  - name: test
    prefix: 'test.'
    mappings:
      - match: "test.invalid.duration"
        match_type: invalid
        name: "test.job.duration"
`,
			packets: []string{
				"test.job.duration.my_job_type.my_job_name",
			},
			expectedError: "invalid match type",
		},
		{
			name: "Missing profile name",
			config: `
dogstatsd_mapper_profiles:
  - prefix: 'test.'
    mappings:
      - match: "test.invalid.duration"
        match_type: invalid
        name: "test.job.duration"
`,
			packets: []string{
				"test.job.duration.my_job_type.my_job_name",
			},
			expectedError: "missing profile name",
		},
		{
			name: "Missing profile prefix",
			config: `
dogstatsd_mapper_profiles:
  - name: test
    mappings:
      - match: "test.invalid.duration"
        match_type: invalid
        name: "test.job.duration"
`,
			packets: []string{
				"test.job.duration.my_job_type.my_job_name",
			},
			expectedError: "missing prefix for profile",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			_, err := getMapper(t, scenario.config)
			require.Error(t, err)
			require.Contains(t, err.Error(), scenario.expectedError)
		})
	}
}

func getMapper(t *testing.T, configString string) (*MetricMapper, error) {
	var profiles []config.MappingProfile

	cfg := fxutil.Test[configComponent.Component](t, fx.Options(
		configComponent.MockModule,
		fx.Replace(configComponent.MockParams{
			Params: configComponent.Params{ConfFilePath: configString},
		}),
	))

	err := cfg.UnmarshalKey("dogstatsd_mapper_profiles", &profiles)
	if err != nil {
		return nil, err
	}
	mapper, err := NewMetricMapper(profiles, 1000)
	if err != nil {
		return nil, err
	}
	return mapper, err
}
