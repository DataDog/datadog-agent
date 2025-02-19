// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseJSONWithValidFormatShouldSucceed(t *testing.T) {
	var configs []*LogsConfig
	var config *LogsConfig
	var err error

	configs, err = ParseJSON([]byte(`[{}]`))
	assert.Nil(t, err)
	config = configs[0]
	assert.NotNil(t, config)

	configs, err = ParseJSON([]byte(`[{"source":"any_source","service":"any_service","tags":["a","b:d"]}]`))
	assert.Nil(t, err)
	config = configs[0]
	assert.Equal(t, "any_source", config.Source)
	assert.Equal(t, "any_service", config.Service)
	assert.EqualValues(t, []string{"a", "b:d"}, config.Tags)

	configs, err = ParseJSON([]byte(`[{"source":"any_source","service":"any_service","log_processing_rules":[{"type":"multi_line","name":"numbers","pattern":"[0-9]"}]}]`))
	assert.Nil(t, err)
	config = configs[0]
	assert.Equal(t, "any_source", config.Source)
	assert.Equal(t, "any_service", config.Service)
	assert.Equal(t, 1, len(config.ProcessingRules))

	rule := config.ProcessingRules[0]
	assert.Equal(t, "multi_line", rule.Type)
	assert.Equal(t, "numbers", rule.Name)
}

func TestParseJSONWithInvalidFormatShouldFail(t *testing.T) {
	invalidFormats := []string{
		"``",
		`{}`,
		`{\"source\":\"any_source\",\"service\":\"any_service\"}`,
	}

	for _, format := range invalidFormats {
		configs, err := ParseJSON([]byte(format))
		assert.NotNil(t, err)
		assert.Nil(t, configs)
	}
}

func TestParseYAMLWithInvalidFormatShouldFail(t *testing.T) {
	invalidFormats := []string{`
foo:
  - type: file
    path: /var/log/app.log
    tags: a, b:c
`, `
- type: file
  path: /var/log/app.log
  tags: a, b:c
`, `
`}

	for _, format := range invalidFormats {
		configs, _ := ParseYAML([]byte(format))
		assert.Equal(t, 0, len(configs))
	}
}

func TestParseYAMLWithValidFormatShouldSucceed(t *testing.T) {
	tests := []struct {
		name   string
		yaml   []byte
		assert func(t *testing.T, configs []*LogsConfig, err error)
	}{
		{
			name: "Test 0: Parse with file logs and multiple tags",
			yaml: []byte(`
logs:
  - type: file
    path: /var/log/app.log
    tags: a, b:c
    container_mode: false
    auto_multi_line_detection: false
    auto_multi_line_match_threshold: 3.0
    port: 8080
`),
			assert: func(t *testing.T, configs []*LogsConfig, err error) {
				assert.Nil(t, err)
				assert.Len(t, configs, 1)
				config := configs[0]
				assert.Equal(t, "file", config.Type)
				assert.Equal(t, "/var/log/app.log", config.Path)
				assert.Len(t, config.Tags, 2)
				assert.Equal(t, "a", config.Tags[0])
				assert.Equal(t, "b:c", config.Tags[1])
			},
		},
		{
			name: "Test 1: Parse with simple include_at_match processing rule",
			yaml: []byte(`
logs:
  - type: file
    path: /my/test/file.log
    service: cardpayment
    source: java
    log_processing_rules:
    - type: include_at_match
      name: include_datadoghq_users
      pattern: \w+@datadoghq.com
`),
			assert: func(t *testing.T, configs []*LogsConfig, err error) {
				assert.Nil(t, err)
				assert.Len(t, configs, 1)
				config := configs[0]
				assert.Equal(t, "file", config.Type)
				assert.Equal(t, "/my/test/file.log", config.Path)
				assert.Equal(t, "cardpayment", config.Service)
				assert.Equal(t, "java", config.Source)
				assert.Len(t, config.ProcessingRules, 1)
				assert.Equal(t, "include_at_match", config.ProcessingRules[0].Type)
				assert.Equal(t, "include_datadoghq_users", config.ProcessingRules[0].Name)
				assert.Equal(t, `\w+@datadoghq.com`, config.ProcessingRules[0].Pattern)
				_, err = regexp.Compile(config.ProcessingRules[0].Pattern)
				assert.Nil(t, err, "Pattern should be a valid regex")
			},
		},
		{
			name: "Test 2: Parse with another pattern and include_at_match processing rule",
			yaml: []byte(`
logs:
  - type: file
    path: /my/test/file.log
    service: cardpayment
    source: java
    log_processing_rules:
    - type: include_at_match
      name: include_datadoghq_users
      pattern: abc|123
`),
			assert: func(t *testing.T, configs []*LogsConfig, err error) {
				assert.Nil(t, err)
				assert.Len(t, configs, 1)
				config := configs[0]
				assert.Equal(t, "file", config.Type)
				assert.Equal(t, "/my/test/file.log", config.Path)
				assert.Equal(t, "cardpayment", config.Service)
				assert.Equal(t, "java", config.Source)
				assert.Len(t, config.ProcessingRules, 1)
				assert.Equal(t, "include_at_match", config.ProcessingRules[0].Type)
				assert.Equal(t, "include_datadoghq_users", config.ProcessingRules[0].Name)
				assert.Equal(t, `abc|123`, config.ProcessingRules[0].Pattern)
				_, err = regexp.Compile(config.ProcessingRules[0].Pattern)
				assert.Nil(t, err, "Pattern should be a valid regex")
			},
		},
		{
			name: "Test 3: Parse with multi-line processing rule",
			yaml: []byte(`
logs:
  - type: file
    path: /var/log/pg_log.log
    service: database
    source: postgresql
    log_processing_rules:
    - type: multi_line
      name: new_log_start_with_date
      pattern: \d{4}\-(0?[1-9]|1[012])\-(0?[1-9]|[12][0-9]|3[01])
`),
			assert: func(t *testing.T, configs []*LogsConfig, err error) {
				assert.Nil(t, err)
				assert.Len(t, configs, 1)
				config := configs[0]
				assert.Equal(t, "file", config.Type)
				assert.Equal(t, "/var/log/pg_log.log", config.Path)
				assert.Equal(t, "database", config.Service)
				assert.Equal(t, "postgresql", config.Source)
				assert.Len(t, config.ProcessingRules, 1)
				assert.Equal(t, "multi_line", config.ProcessingRules[0].Type)
				assert.Equal(t, "new_log_start_with_date", config.ProcessingRules[0].Name)
				assert.Equal(t, `\d{4}\-(0?[1-9]|1[012])\-(0?[1-9]|[12][0-9]|3[01])`, config.ProcessingRules[0].Pattern)
				_, err = regexp.Compile(config.ProcessingRules[0].Pattern)
				assert.Nil(t, err, "Pattern should be a valid regex")
			},
		},
		{
			name: "Test 4: Parse with mask_sequences processing rule",
			yaml: []byte(`
logs:
 - type: file
   path: /my/test/file.log
   service: cardpayment
   source: java
   log_processing_rules:
      - type: mask_sequences
        name: mask_credit_cards
        replace_placeholder: "[masked_credit_card]"
        pattern: (?:4[0-9]{12}(?:[0-9]{3})?|[25][1-7][0-9]{14}|6(?:011|5[0-9][0-9])[0-9]{12}|3[47][0-9]{13}|3(?:0[0-5]|[68][0-9])[0-9]{11}|(?:2131|1800|35\d{3})\d{11})
`),
			assert: func(t *testing.T, configs []*LogsConfig, err error) {
				assert.Nil(t, err)
				assert.Len(t, configs, 1)
				config := configs[0]
				assert.Equal(t, "file", config.Type)
				assert.Equal(t, "/my/test/file.log", config.Path)
				assert.Equal(t, "cardpayment", config.Service)
				assert.Equal(t, "java", config.Source)
				assert.Len(t, config.ProcessingRules, 1)
				assert.Equal(t, "mask_sequences", config.ProcessingRules[0].Type)
				assert.Equal(t, "mask_credit_cards", config.ProcessingRules[0].Name)
				assert.Equal(t, "[masked_credit_card]", config.ProcessingRules[0].ReplacePlaceholder)
				assert.Equal(t, `(?:4[0-9]{12}(?:[0-9]{3})?|[25][1-7][0-9]{14}|6(?:011|5[0-9][0-9])[0-9]{12}|3[47][0-9]{13}|3(?:0[0-5]|[68][0-9])[0-9]{11}|(?:2131|1800|35\d{3})\d{11})`, config.ProcessingRules[0].Pattern)
				_, err = regexp.Compile(config.ProcessingRules[0].Pattern)
				assert.Nil(t, err, "Pattern should be a valid regex")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configs, err := ParseYAML(tt.yaml)
			tt.assert(t, configs, err)
		})
	}
}
