// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package mapper

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sort"
	"strings"
	"testing"
)

type MappingResult struct {
	Name    string
	Tags    []string
	Matched bool
}

func TestMappings(t *testing.T) {
	scenarios := []struct {
		name            string
		config          string
		packets         []string
		expectedResults []MappingResult
	}{
		{
			name: "Simple OK case",
			config: `
dogstatsd_mappings:
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
			expectedResults: []MappingResult{
				{Name: "test.job.duration", Tags: []string{"job_type:my_job_type", "job_name:my_job_name"}, Matched: true},
				{Name: "test.job.size", Tags: []string{"foo:my_job_type", "bar:my_job_name"}, Matched: true},
				{Name: "", Tags: nil, Matched: false},
			},
		},
		{
			name: "Not/Partially mapped are accepted",
			config: `
dogstatsd_mappings:
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
			expectedResults: []MappingResult{
				{Name: "test.job.duration", Tags: []string{"job_type:my_job_type"}, Matched: true},
				{Name: "test.task.duration", Tags: nil, Matched: true},
			},
		},
		{
			name: "Use regex expansion alternative syntax",
			config: `
dogstatsd_mappings:
 - match: "test.job.duration.*.*"
   name: "test.job.duration"
   tags:
     job_type: "${1}_x"
     job_name: "${2}_y"
`,
			packets: []string{
				"test.job.duration.my_job_type.my_job_name",
			},
			expectedResults: []MappingResult{
				{Name: "test.job.duration", Tags: []string{"job_type:my_job_type_x", "job_name:my_job_name_y"}, Matched: true},
			},
		},
		{
			name: "Expand name",
			config: `
dogstatsd_mappings:
 - match: "test.job.duration.*.*"
   name: "test.hello.$2.$1"
   tags:
     job_type: "$1"
     job_name: "$2"
`,
			packets: []string{
				"test.job.duration.my_job_type.my_job_name",
			},
			expectedResults: []MappingResult{
				{Name: "test.hello.my_job_name.my_job_type", Tags: []string{"job_type:my_job_type", "job_name:my_job_name"}, Matched: true},
			},
		},
		{
			name: "Match before underscore",
			config: `
dogstatsd_mappings:
 - match: "test.*_start"
   name: "test.start"
   tags:
     job: "$1"
`,
			packets: []string{
				"test.my_job_start",
			},
			expectedResults: []MappingResult{
				{Name: "test.start", Tags: []string{"job:my_job"}, Matched: true},
			},
		},
		{
			name: "No tags",
			config: `
dogstatsd_mappings:
 - match: "test.my-worker.start"
   name: "test.worker.start"
 - match: "test.my-worker.stop.*"
   name: "test.worker.stop"
`,
			packets: []string{
				"test.my-worker.start",
				"test.my-worker.stop.worker-name",
			},
			expectedResults: []MappingResult{
				{Name: "test.worker.start", Tags: nil, Matched: true},
				{Name: "test.worker.stop", Tags: nil, Matched: true},
			},
		},
		{
			name: "All allowed characters",
			config: `
dogstatsd_mappings:
 - match: "test.abcdefghijklmnopqrstuvwxyz_ABCDEFGHIJKLMNOPQRSTUVWXYZ-01234567.*"
   name: "test.alphabet"
`,
			packets: []string{
				"test.abcdefghijklmnopqrstuvwxyz_ABCDEFGHIJKLMNOPQRSTUVWXYZ-01234567.123",
			},
			expectedResults: []MappingResult{
				{Name: "test.alphabet", Tags: nil, Matched: true},
			},
		},
		{
			name: "Regex match type",
			config: `
dogstatsd_mappings:
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
			expectedResults: []MappingResult{
				{Name: "test.job.duration", Tags: []string{"job_name:my.funky.job$name-abc/123"}, Matched: true},
				{Name: "test.task.duration", Tags: []string{"task_name:MY_task_name"}, Matched: true},
			},
		},
		{
			name: "Complex Regex match type",
			config: `
dogstatsd_mappings:
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
			expectedResults: []MappingResult{
				{Name: "test.job", Tags: []string{"job_type:a5-foo", "job_name:bar"}, Matched: true},
				{Name: "", Tags: nil, Matched: false},
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			mapper, err := getMapper(scenario.config)
			require.NoError(t, err)

			var actualResults []MappingResult
			for _, packet := range scenario.packets {
				name, tags, matched := mapper.GetMapping(packet)
				actualResults = append(actualResults, MappingResult{Name: name, Tags: tags, Matched: matched})
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
dogstatsd_mappings:
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
dogstatsd_mappings:
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
dogstatsd_mappings:
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
dogstatsd_mappings:
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
dogstatsd_mappings:
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
dogstatsd_mappings:
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
dogstatsd_mappings:
 - match: "test.invalid.duration"
   match_type: invalid
   name: "test.job.duration"
`,
			packets: []string{
				"test.job.duration.my_job_type.my_job_name",
			},
			expectedError: "invalid match type",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			_, err := getMapper(scenario.config)
			require.Error(t, err)
			require.Contains(t, err.Error(), scenario.expectedError)
		})
	}
}

func getMapper(configString string) (*MetricMapper, error) {
	var mappings []MetricMapping
	config.Datadog.SetConfigType("yaml")
	err := config.Datadog.ReadConfig(strings.NewReader(configString))
	if err != nil {
		return nil, err
	}
	err = config.Datadog.UnmarshalKey("dogstatsd_mappings", &mappings)
	if err != nil {
		return nil, err
	}
	mapper, err := NewMetricMapper(mappings, 1000)
	if err != nil {
		return nil, err
	}
	return &mapper, err
}
