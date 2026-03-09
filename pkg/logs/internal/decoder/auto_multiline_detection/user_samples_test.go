// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestUserPatternsInvalidConfig(t *testing.T) {
	datadogYaml := `
logs_config:
  auto_multi_line_detection_custom_samples:
    - foo bar
`
	mockConfig := mock.NewFromYAML(t, datadogYaml)

	samples := NewUserSamples(mockConfig, nil)
	assert.Equal(t, 0, len(samples.samples))
}

func TestUserPatternsInvalidValues(t *testing.T) {
	datadogYaml := `
logs_config:
  auto_multi_line_detection_custom_samples:
    - sample: 1234
      match_threshold: "abcd"
      label: foo bar
`
	mockConfig := mock.NewFromYAML(t, datadogYaml)

	samples := NewUserSamples(mockConfig, nil)
	assert.Equal(t, 0, len(samples.samples))
}

func TestUserPatternsDefaults(t *testing.T) {

	expectedOutput, _ := NewTokenizer(0).tokenize([]byte("sample"))

	datadogYaml := `
logs_config:
  auto_multi_line_detection_custom_samples:
    - sample: "sample"
`
	mockConfig := mock.NewFromYAML(t, datadogYaml)

	samples := NewUserSamples(mockConfig, nil)
	assert.Equal(t, expectedOutput, samples.samples[0].tokens)
	assert.Equal(t, defaultMatchThreshold, samples.samples[0].matchThreshold)
	assert.Equal(t, startGroup, samples.samples[0].label)
}

func TestUserPatternsLabelTypes(t *testing.T) {

	datadogYaml := `
logs_config:
  auto_multi_line_detection_custom_samples:
    - sample: "1"
      label: "start_group"
    - sample: "2"
      label: "no_aggregate"
    - sample: "3"
      label: "aggregate"
    - sample: "4"
      label: "invalid"
`
	mockConfig := mock.NewFromYAML(t, datadogYaml)

	samples := NewUserSamples(mockConfig, nil)
	assert.Equal(t, 3, len(samples.samples))
	assert.Equal(t, startGroup, samples.samples[0].label)
	assert.Equal(t, noAggregate, samples.samples[1].label)
	assert.Equal(t, aggregate, samples.samples[2].label)
}

func TestUserPatternsJSON(t *testing.T) {

	mockConfig := mock.New(t)
	mockConfig.SetWithoutSource("logs_config.auto_multi_line_detection_custom_samples", `[{"sample": "1", "label": "start_group"}, {"regex": "\\d\\w", "label": "no_aggregate"}, {"sample": "3", "match_threshold": 0.1}]`)

	sampleOneTokens, _ := NewTokenizer(0).tokenize([]byte("1"))
	sampleTwoRegex, _ := regexp.Compile("^" + "\\d\\w")
	sampleThreeTokens, _ := NewTokenizer(0).tokenize([]byte("3"))
	samples := NewUserSamples(mockConfig, nil)
	assert.Equal(t, 3, len(samples.samples))
	assert.Equal(t, startGroup, samples.samples[0].label)
	assert.Equal(t, sampleOneTokens, samples.samples[0].tokens)
	assert.Equal(t, noAggregate, samples.samples[1].label)
	assert.Equal(t, sampleTwoRegex, samples.samples[1].compiledRegex)
	assert.Equal(t, sampleThreeTokens, samples.samples[2].tokens)
	assert.Equal(t, 0.1, samples.samples[2].matchThreshold)
}

func TestUserPatternsJSONEnv(t *testing.T) {

	mockConfig := mock.New(t)
	t.Setenv("DD_LOGS_CONFIG_AUTO_MULTI_LINE_DETECTION_CUSTOM_SAMPLES", `[{"sample": "1", "label": "start_group"}, {"regex": "\\d\\w", "label": "no_aggregate"}, {"sample": "3", "match_threshold": 0.1}]`)

	sampleOneTokens, _ := NewTokenizer(0).tokenize([]byte("1"))
	sampleTwoRegex, _ := regexp.Compile("^" + "\\d\\w")
	sampleThreeTokens, _ := NewTokenizer(0).tokenize([]byte("3"))
	samples := NewUserSamples(mockConfig, nil)
	assert.Equal(t, 3, len(samples.samples))
	assert.Equal(t, startGroup, samples.samples[0].label)
	assert.Equal(t, sampleOneTokens, samples.samples[0].tokens)
	assert.Equal(t, noAggregate, samples.samples[1].label)
	assert.Equal(t, sampleTwoRegex, samples.samples[1].compiledRegex)
	assert.Equal(t, sampleThreeTokens, samples.samples[2].tokens)
	assert.Equal(t, 0.1, samples.samples[2].matchThreshold)
}

func TestUserPatternsMatchThreshold(t *testing.T) {

	datadogYaml := `
logs_config:
  auto_multi_line_detection_custom_samples:
    - sample: "default"
    - sample: "custom"
      match_threshold: 0.1234
    - sample: "invalid1"
      match_threshold: -9241
    - sample: "invalid2"
      match_threshold: 2
    - sample:
    - sample: ""
    - match_threshold: 0.1
    - label: no_aggregate
`
	mockConfig := mock.NewFromYAML(t, datadogYaml)

	samples := NewUserSamples(mockConfig, nil)
	assert.Equal(t, 2, len(samples.samples))
	assert.Equal(t, defaultMatchThreshold, samples.samples[0].matchThreshold)
	assert.Equal(t, 0.1234, samples.samples[1].matchThreshold)
}

func TestUserPatternsProcess(t *testing.T) {

	datadogYaml := `
logs_config:
  auto_multi_line_detection_custom_samples:
    - sample: "!!![$my custom prefix%]"
`

	mockConfig := mock.NewFromYAML(t, datadogYaml)
	samples := NewUserSamples(mockConfig, nil)
	tokenizer := NewTokenizer(60)

	tests := []struct {
		expectedLabel Label
		shouldStop    bool
		input         string
	}{
		{aggregate, true, ""},
		{aggregate, true, "some random log line"},
		{aggregate, true, "2023-03-28T14:33:53.743350Z App started successfully"},
		{startGroup, false, "!!![$my custom prefix%] some other log line"},
		{startGroup, false, "!!![$my slight variation%] some other log line"},
		{aggregate, true, "!!![$Not_close_enough%] some other log line"},
	}

	for _, test := range tests {
		context := &messageContext{
			rawMessage: []byte(test.input),
			label:      aggregate,
		}

		assert.True(t, tokenizer.ProcessAndContinue(context))
		assert.Equal(t, test.shouldStop, samples.ProcessAndContinue(context), "Expected stop %v, got %v", test.shouldStop, samples.ProcessAndContinue(context))
		assert.Equal(t, test.expectedLabel, context.label, "Expected label %v, got %v", test.expectedLabel, context.label)
	}
}

func TestUserPatternsProcessCustomSettings(t *testing.T) {

	datadogYaml := `
logs_config:
  auto_multi_line_detection_custom_samples:
    - sample: "!!![$my custom prefix%]"
      match_threshold: 0.1
      label: no_aggregate
`

	mockConfig := mock.NewFromYAML(t, datadogYaml)
	samples := NewUserSamples(mockConfig, nil)
	tokenizer := NewTokenizer(60)

	tests := []struct {
		expectedLabel Label
		shouldStop    bool
		input         string
	}{
		{aggregate, true, ""},
		{aggregate, true, "some random log line"},
		{aggregate, true, "2023-03-28T14:33:53.743350Z App started successfully"},
		{noAggregate, false, "!!![$my custom prefix%] some other log line"},
		{noAggregate, false, "!!![$my slight variation%] some other log line"},
		{noAggregate, false, "!!![$Not_close_enough%] some other log line"}, // Now this case works with a lower match threshold
	}

	for _, test := range tests {
		context := &messageContext{
			rawMessage: []byte(test.input),
			label:      aggregate,
		}

		assert.True(t, tokenizer.ProcessAndContinue(context))
		assert.Equal(t, test.shouldStop, samples.ProcessAndContinue(context), "Expected stop %v, got %v", test.shouldStop, samples.ProcessAndContinue(context))
		assert.Equal(t, test.expectedLabel, context.label, "Expected label %v, got %v", test.expectedLabel, context.label)
	}
}

func TestUserPatternsRegexProcess(t *testing.T) {

	datadogYaml := `
logs_config:
  auto_multi_line_extra_patterns:
    - "le\\wacy"
  auto_multi_line_detection_custom_samples:
    - regex: "(foo|bar)test\\d+"
`

	mockConfig := mock.NewFromYAML(t, datadogYaml)
	samples := NewUserSamples(mockConfig, nil)
	tokenizer := NewTokenizer(60)

	tests := []struct {
		expectedLabel Label
		shouldStop    bool
		input         string
	}{
		{aggregate, true, ""},
		{aggregate, true, "some random log line"},
		{aggregate, true, "2023-03-28T14:33:53.743350Z App started successfully"},
		{startGroup, false, "footest123 some other log line"},
		{startGroup, false, "bartest123 some other log line"},
		{startGroup, false, "legacy pattern should match me"},
		{aggregate, true, "!!![$Not_close_enough%] some other log line"},
	}

	for _, test := range tests {
		context := &messageContext{
			rawMessage: []byte(test.input),
			label:      aggregate,
		}

		assert.True(t, tokenizer.ProcessAndContinue(context))
		assert.Equal(t, test.shouldStop, samples.ProcessAndContinue(context), "Expected stop %v, got %v", test.shouldStop, samples.ProcessAndContinue(context))
		assert.Equal(t, test.expectedLabel, context.label, "Expected label %v, got %v", test.expectedLabel, context.label)
	}
}

func TestOnlyLegacyPatterns(t *testing.T) {

	datadogYaml := `
logs_config:
  auto_multi_line_extra_patterns:
    - "le\\wacy"
`

	mockConfig := mock.NewFromYAML(t, datadogYaml)
	samples := NewUserSamples(mockConfig, nil)
	tokenizer := NewTokenizer(60)

	tests := []struct {
		expectedLabel Label
		shouldStop    bool
		input         string
	}{
		{startGroup, false, "legacy pattern should match me"},
		{startGroup, false, "leAacy pattern should match me"},
		{startGroup, false, "leVacy pattern should match me"},
	}

	for _, test := range tests {
		context := &messageContext{
			rawMessage: []byte(test.input),
			label:      aggregate,
		}

		assert.True(t, tokenizer.ProcessAndContinue(context))
		assert.Equal(t, test.shouldStop, samples.ProcessAndContinue(context), "Expected stop %v, got %v", test.shouldStop, samples.ProcessAndContinue(context))
		assert.Equal(t, test.expectedLabel, context.label, "Expected label %v, got %v", test.expectedLabel, context.label)
	}
}

func TestUserPatternsProcessRegexCustomSettings(t *testing.T) {

	datadogYaml := `
logs_config:
  auto_multi_line_detection_custom_samples:
    - regex: "(foo|bar)test\\d+"
      label: no_aggregate
`

	mockConfig := mock.NewFromYAML(t, datadogYaml)
	samples := NewUserSamples(mockConfig, nil)
	tokenizer := NewTokenizer(60)

	tests := []struct {
		expectedLabel Label
		shouldStop    bool
		input         string
	}{
		{aggregate, true, ""},
		{aggregate, true, "some random log line"},
		{aggregate, true, "2023-03-28T14:33:53.743350Z App started successfully"},
		{noAggregate, false, "footest123 some other log line"},
		{noAggregate, false, "bartest123 some other log line"},
	}

	for _, test := range tests {
		context := &messageContext{
			rawMessage: []byte(test.input),
			label:      aggregate,
		}

		assert.True(t, tokenizer.ProcessAndContinue(context))
		assert.Equal(t, test.shouldStop, samples.ProcessAndContinue(context), "Expected stop %v, got %v", test.shouldStop, samples.ProcessAndContinue(context))
		assert.Equal(t, test.expectedLabel, context.label, "Expected label %v, got %v", test.expectedLabel, context.label)
	}
}

func TestUserPatternsWithIntegrationSamples(t *testing.T) {
	expectedOutput, _ := NewTokenizer(0).tokenize([]byte("sample"))
	rawSamples := []*config.AutoMultilineSample{
		{Sample: "sample"},
	}

	mockConfig := mock.NewFromYAML(t, "")

	samples := NewUserSamples(mockConfig, rawSamples)
	assert.Equal(t, expectedOutput, samples.samples[0].tokens)
	assert.Equal(t, defaultMatchThreshold, samples.samples[0].matchThreshold)
}

func TestUserPatternsWithIntegrationSamplesCollection(t *testing.T) {
	noAggregateString := "no_aggregate"
	aggregateString := "aggregate"
	rawSamples := []*config.AutoMultilineSample{
		{Sample: "sample"},
		{Sample: "skip_me", Label: &noAggregateString},
		{Regex: "regex", Label: &aggregateString},
	}

	mockConfig := mock.NewFromYAML(t, "")
	samples := NewUserSamples(mockConfig, rawSamples)
	tokenizer := NewTokenizer(60)

	tests := []struct {
		expectedLabel Label
		shouldStop    bool
		input         string
	}{
		{startGroup, false, "sample"},
		{aggregate, true, "some random log line"},
		{aggregate, true, "2023-03-28T14:33:53.743350Z App started successfully"},
		{startGroup, false, "sample"},
		{noAggregate, false, "skip_me"},
		{aggregate, false, "regex"},
	}

	for _, test := range tests {
		context := &messageContext{
			rawMessage: []byte(test.input),
			label:      aggregate,
		}

		assert.True(t, tokenizer.ProcessAndContinue(context))
		assert.Equal(t, test.shouldStop, samples.ProcessAndContinue(context), "Expected stop %v, got %v", test.shouldStop, samples.ProcessAndContinue(context))
		assert.Equal(t, test.expectedLabel, context.label, "Expected label %v, got %v", test.expectedLabel, context.label)
	}
}

func TestUserPatternWithIntegrationSampleMatchThreshold(t *testing.T) {
	matchThreshold := 0.6
	rawSamples := []*config.AutoMultilineSample{
		{Sample: "12345", MatchThreshold: &matchThreshold},
	}

	mockConfig := mock.NewFromYAML(t, "")
	samples := NewUserSamples(mockConfig, rawSamples)
	tokenizer := NewTokenizer(60)

	tests := []struct {
		expectedLabel Label
		shouldStop    bool
		input         string
	}{
		{startGroup, false, "12345"},
		{aggregate, true, "12"},
		{startGroup, false, "12345"},
	}

	for _, test := range tests {
		context := &messageContext{
			rawMessage: []byte(test.input),
			label:      aggregate,
		}

		assert.True(t, tokenizer.ProcessAndContinue(context))
		assert.Equal(t, test.shouldStop, samples.ProcessAndContinue(context), "Expected stop %v, got %v", test.shouldStop, samples.ProcessAndContinue(context))
		assert.Equal(t, test.expectedLabel, context.label, "Expected label %v, got %v", test.expectedLabel, context.label)
	}

}
