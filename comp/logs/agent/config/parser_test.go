// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
  tags: nil
`, `
`}

	for i, format := range invalidFormats {
		configs, err := ParseYAML([]byte(format))
		if i == 1 {
			assert.NotNil(t, err)
		}
		require.Equal(t, 0, len(configs))
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
				require.Equal(t, len(configs), 1)
				config := configs[0]
				assert.Equal(t, "file", config.Type)
				assert.Equal(t, "/var/log/app.log", config.Path)
				require.Equal(t, len(config.Tags), 2)
				assert.Equal(t, "a", config.Tags[0])
				assert.Equal(t, " b:c", config.Tags[1])
			},
		},
		{
			name: "Test 0.5: Same as Test 0, but without string separation",
			yaml: []byte(`
logs:
  - type: file
    path: /var/log/app.log
    tags: a,b:c
    container_mode: false
    auto_multi_line_detection: false
    auto_multi_line_match_threshold: 3.0
    port: 8080
`),
			assert: func(t *testing.T, configs []*LogsConfig, err error) {
				assert.Nil(t, err)
				require.Equal(t, len(configs), 1)
				config := configs[0]
				assert.Equal(t, "file", config.Type)
				assert.Equal(t, "/var/log/app.log", config.Path)
				require.Equal(t, len(config.Tags), 2)
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
				require.Equal(t, len(configs), 1)
				config := configs[0]
				assert.Equal(t, "file", config.Type)
				assert.Equal(t, "/my/test/file.log", config.Path)
				assert.Equal(t, "cardpayment", config.Service)
				assert.Equal(t, "java", config.Source)
				require.Equal(t, len(config.ProcessingRules), 1)
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
				require.Equal(t, len(configs), 1)
				config := configs[0]
				assert.Equal(t, "file", config.Type)
				assert.Equal(t, "/my/test/file.log", config.Path)
				assert.Equal(t, "cardpayment", config.Service)
				assert.Equal(t, "java", config.Source)
				require.Equal(t, len(config.ProcessingRules), 1)
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
				require.Equal(t, len(configs), 1)
				config := configs[0]
				assert.Equal(t, "file", config.Type)
				assert.Equal(t, "/var/log/pg_log.log", config.Path)
				assert.Equal(t, "database", config.Service)
				assert.Equal(t, "postgresql", config.Source)
				require.Equal(t, len(config.ProcessingRules), 1)
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
				require.Equal(t, len(configs), 1)
				config := configs[0]
				assert.Equal(t, "file", config.Type)
				assert.Equal(t, "/my/test/file.log", config.Path)
				assert.Equal(t, "cardpayment", config.Service)
				assert.Equal(t, "java", config.Source)
				require.Equal(t, len(config.ProcessingRules), 1)
				assert.Equal(t, "mask_sequences", config.ProcessingRules[0].Type)
				assert.Equal(t, "mask_credit_cards", config.ProcessingRules[0].Name)
				assert.Equal(t, "[masked_credit_card]", config.ProcessingRules[0].ReplacePlaceholder)
				assert.Equal(t, `(?:4[0-9]{12}(?:[0-9]{3})?|[25][1-7][0-9]{14}|6(?:011|5[0-9][0-9])[0-9]{12}|3[47][0-9]{13}|3(?:0[0-5]|[68][0-9])[0-9]{11}|(?:2131|1800|35\d{3})\d{11})`, config.ProcessingRules[0].Pattern)
				_, err = regexp.Compile(config.ProcessingRules[0].Pattern)
				assert.Nil(t, err, "Pattern should be a valid regex")
			},
		},
		{
			name: "Test 5: Parse journald with include_units (string with newline)",
			yaml: []byte(`
logs:
  - type: journald
    path: /var/log/journal/
    source: custom_log
    service: random-logger
    include_units:
      random-logger.service
    default_application_name: random-logger
`),
			assert: func(t *testing.T, configs []*LogsConfig, err error) {
				assert.Nil(t, err)
				require.Equal(t, len(configs), 1)
				config := configs[0]
				assert.Equal(t, "journald", config.Type)
				assert.Equal(t, "/var/log/journal/", config.Path)
				assert.Equal(t, "custom_log", config.Source)
				assert.Equal(t, "random-logger", config.Service)
				require.Equal(t, len(config.IncludeSystemUnits), 1)
				assert.Equal(t, "random-logger.service", config.IncludeSystemUnits[0])
				assert.NotNil(t, config.DefaultApplicationName)
				assert.Equal(t, "random-logger", *config.DefaultApplicationName)
			},
		},
		{
			name: "Test 6: Parse journald with exclude_units",
			yaml: []byte(`
logs:
  - type: journald
    source: custom_log
    service: no-datadog
    exclude_units:
      datadog-agent.service,datadog-agent-trace.service,datadog-agent-process.service
`),
			assert: func(t *testing.T, configs []*LogsConfig, err error) {
				assert.Nil(t, err)
				require.Equal(t, len(configs), 1)
				config := configs[0]
				assert.Equal(t, "journald", config.Type)
				assert.Equal(t, "custom_log", config.Source)
				assert.Equal(t, "no-datadog", config.Service)
				require.Equal(t, len(config.ExcludeSystemUnits), 3)
				assert.Equal(t, "datadog-agent.service", config.ExcludeSystemUnits[0])
				assert.Equal(t, "datadog-agent-trace.service", config.ExcludeSystemUnits[1])
				assert.Equal(t, "datadog-agent-process.service", config.ExcludeSystemUnits[2])
			},
		},
		{
			name: "Test 7: Parse minimal journald config",
			yaml: []byte(`
logs:
  - type: journald
    service: hello
    source: custom_log
`),
			assert: func(t *testing.T, configs []*LogsConfig, err error) {
				assert.Nil(t, err)
				require.Equal(t, len(configs), 1)
				config := configs[0]
				assert.Equal(t, "journald", config.Type)
				assert.Equal(t, "hello", config.Service)
				assert.Equal(t, "custom_log", config.Source)
				assert.Nil(t, config.DefaultApplicationName)
			},
		},
		{
			name: "Test Comprehensive: All parseable fields in LogsConfig",
			yaml: []byte(`
logs:
  - type: journald
    integrationname: test_integration
    port: 8080
    idle_timeout: "30s"
    path: /var/log/journal/
    encoding: utf-8
    exclude_paths:
      - /var/log/exclude1.log
      - /var/log/exclude2.log
    start_position: beginning
    config_id: test_config_id
    include_units:
      - systemd-unit.service
    exclude_units:
      - datadog-agent.service
      - datadog-agent-trace.service
      - datadog-agent-process.service
    include_user_units:
      - user-systemd-unit.service
    exclude_user_units:
      - user-exclude.service
    include_matches:
      - some_match
    exclude_matches:
      - some_exclude_match
    container_mode: true
    default_application_name: "container-runtime"
    image: test_image
    label: test_label
    name: test_container
    identifier: test_identifier
    channel_path: /test/channel
    query: SELECT * FROM logs
    service: test_service
    source: test_source
    sourcecategory: test_category
    tags:
      - key:value
      - another_key:another_value
    log_processing_rules:
      - type: include_at_match
        name: include_example
        pattern: example_pattern
    process_raw_message: true
    auto_multi_line_detection: true
    auto_multi_line_sample_size: 5
    auto_multi_line_match_threshold: 2.5
`),
			assert: func(t *testing.T, configs []*LogsConfig, err error) {
				assert.Nil(t, err)
				require.Equal(t, len(configs), 1)
				config := configs[0]
				assert.Equal(t, "journald", config.Type)
				assert.Equal(t, "test_integration", config.IntegrationName)
				assert.Equal(t, 8080, config.Port)
				assert.Equal(t, "30s", config.IdleTimeout)
				assert.Equal(t, "/var/log/journal/", config.Path)
				assert.Equal(t, "utf-8", config.Encoding)
				require.Equal(t, len(config.ExcludePaths), 2)
				assert.Equal(t, "beginning", config.TailingMode)
				assert.Equal(t, "test_config_id", config.ConfigID)
				require.Equal(t, len(config.IncludeSystemUnits), 1)
				require.Equal(t, len(config.ExcludeSystemUnits), 3)
				require.Equal(t, len(config.IncludeUserUnits), 1)
				require.Equal(t, len(config.ExcludeUserUnits), 1)
				require.Equal(t, len(config.IncludeMatches), 1)
				require.Equal(t, len(config.ExcludeMatches), 1)
				assert.Equal(t, true, config.ContainerMode)
				require.NotNil(t, config.DefaultApplicationName, "DefaultApplicationName should be set")
				assert.Equal(t, "container-runtime", *config.DefaultApplicationName)
				assert.Equal(t, "test_image", config.Image)
				assert.Equal(t, "test_label", config.Label)
				assert.Equal(t, "test_container", config.Name)
				assert.Equal(t, "test_identifier", config.Identifier)
				assert.Equal(t, "/test/channel", config.ChannelPath)
				assert.Equal(t, "SELECT * FROM logs", config.Query)
				assert.Equal(t, "test_service", config.Service)
				assert.Equal(t, "test_source", config.Source)
				assert.Equal(t, "test_category", config.SourceCategory)
				require.Equal(t, len(config.Tags), 2)
				require.Equal(t, len(config.ProcessingRules), 1)
				assert.Equal(t, "include_at_match", config.ProcessingRules[0].Type)
				assert.Equal(t, "include_example", config.ProcessingRules[0].Name)
				assert.Equal(t, "example_pattern", config.ProcessingRules[0].Pattern)
				assert.Equal(t, true, *config.ProcessRawMessage)
				assert.Equal(t, true, *config.AutoMultiLine)
				assert.Equal(t, 5, config.AutoMultiLineSampleSize)
				assert.Equal(t, 2.5, config.AutoMultiLineMatchThreshold)
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

func TestParseJSONOrYAML(t *testing.T) {
	tests := []struct {
		name   string
		data   []byte
		assert func(t *testing.T, configs []*LogsConfig, err error)
	}{
		{
			name: "Test JSON parsing succeeds first",
			data: []byte(`[{"source":"json_source","service":"json_service","tags":["json_tag"]}]`),
			assert: func(t *testing.T, configs []*LogsConfig, err error) {
				assert.Nil(t, err)
				require.Equal(t, len(configs), 1)
				config := configs[0]
				assert.Equal(t, "json_source", config.Source)
				assert.Equal(t, "json_service", config.Service)
				require.Equal(t, len(config.Tags), 1)
				assert.Equal(t, "json_tag", config.Tags[0])
			},
		},
		{
			name: "Test YAML parsing succeeds when JSON fails",
			data: []byte(`
logs:
  - type: file
    path: /var/log/app.log
    source: yaml_source
    service: yaml_service
    tags: yaml_tag
`),
			assert: func(t *testing.T, configs []*LogsConfig, err error) {
				assert.Nil(t, err)
				require.Equal(t, len(configs), 1)
				config := configs[0]
				assert.Equal(t, "file", config.Type)
				assert.Equal(t, "/var/log/app.log", config.Path)
				assert.Equal(t, "yaml_source", config.Source)
				assert.Equal(t, "yaml_service", config.Service)
				require.Equal(t, len(config.Tags), 1)
				assert.Equal(t, "yaml_tag", config.Tags[0])
			},
		},
		{
			name: "Test multiple configs in JSON",
			data: []byte(`[{"source":"source1","service":"service1"},{"source":"source2","service":"service2"}]`),
			assert: func(t *testing.T, configs []*LogsConfig, err error) {
				assert.Nil(t, err)
				require.Equal(t, len(configs), 2)
				assert.Equal(t, "source1", configs[0].Source)
				assert.Equal(t, "service1", configs[0].Service)
				assert.Equal(t, "source2", configs[1].Source)
				assert.Equal(t, "service2", configs[1].Service)
			},
		},
		{
			name: "Test multiple configs in YAML when JSON fails",
			data: []byte(`
logs:
  - type: file
    path: /var/log/app1.log
    source: source1
    service: service1
  - type: file
    path: /var/log/app2.log
    source: source2
    service: service2
`),
			assert: func(t *testing.T, configs []*LogsConfig, err error) {
				assert.Nil(t, err)
				require.Equal(t, len(configs), 2)
				assert.Equal(t, "file", configs[0].Type)
				assert.Equal(t, "/var/log/app1.log", configs[0].Path)
				assert.Equal(t, "source1", configs[0].Source)
				assert.Equal(t, "service1", configs[0].Service)
				assert.Equal(t, "file", configs[1].Type)
				assert.Equal(t, "/var/log/app2.log", configs[1].Path)
				assert.Equal(t, "source2", configs[1].Source)
				assert.Equal(t, "service2", configs[1].Service)
			},
		},
		{
			name: "Test empty JSON array",
			data: []byte(`[]`),
			assert: func(t *testing.T, configs []*LogsConfig, err error) {
				assert.Nil(t, err)
				assert.Equal(t, len(configs), 0)
			},
		},
		{
			name: "Test empty YAML logs array when JSON fails",
			data: []byte(`
logs: []
`),
			assert: func(t *testing.T, configs []*LogsConfig, err error) {
				assert.Nil(t, err)
				assert.Equal(t, len(configs), 0)
			},
		},
		{
			name: "Test invalid input that fails both JSON and YAML",
			data: []byte(`invalid data that is neither json nor yaml`),
			assert: func(t *testing.T, configs []*LogsConfig, err error) {
				assert.NotNil(t, err)
				assert.Nil(t, configs)
				assert.Contains(t, err.Error(), "could not parse logs config as JSON or YAML")
			},
		},
		{
			name: "Test YAML without logs wrapper that fails both",
			data: []byte(`
- type: file
  path: /var/log/app.log
`),
			assert: func(t *testing.T, configs []*LogsConfig, err error) {
				// This should fail both JSON and YAML because:
				// - JSON expects array of LogsConfig objects
				// - YAML expects "logs:" wrapper
				assert.NotNil(t, err)
				assert.Nil(t, configs)
				assert.Contains(t, err.Error(), "could not parse logs config as JSON or YAML")
			},
		},
		{
			name: "Test JSON with empty object in array",
			data: []byte(`[{}]`),
			assert: func(t *testing.T, configs []*LogsConfig, err error) {
				assert.Nil(t, err)
				require.Equal(t, len(configs), 1)
				assert.NotNil(t, configs[0])
			},
		},
		{
			name: "Test YAML with empty object when JSON fails",
			data: []byte(`
logs:
  - {}
`),
			assert: func(t *testing.T, configs []*LogsConfig, err error) {
				assert.Nil(t, err)
				require.Equal(t, len(configs), 1)
				assert.NotNil(t, configs[0])
			},
		},
		{
			name: "Test comprehensive JSON config",
			data: []byte(`[{"type":"file","path":"/var/log/test.log","source":"test_source","service":"test_service","tags":["tag1","tag2"],"encoding":"utf-8","port":8080,"container_mode":true,"log_processing_rules":[{"type":"multi_line","name":"test_rule","pattern":"^\\d{4}"}]}]`),
			assert: func(t *testing.T, configs []*LogsConfig, err error) {
				assert.Nil(t, err)
				require.Equal(t, len(configs), 1)
				config := configs[0]
				assert.Equal(t, "file", config.Type)
				assert.Equal(t, "/var/log/test.log", config.Path)
				assert.Equal(t, "test_source", config.Source)
				assert.Equal(t, "test_service", config.Service)
				require.Equal(t, len(config.Tags), 2)
				assert.Equal(t, "tag1", config.Tags[0])
				assert.Equal(t, "tag2", config.Tags[1])
				assert.Equal(t, "utf-8", config.Encoding)
				assert.Equal(t, 8080, config.Port)
				assert.Equal(t, true, config.ContainerMode)
				require.Equal(t, len(config.ProcessingRules), 1)
				assert.Equal(t, "multi_line", config.ProcessingRules[0].Type)
				assert.Equal(t, "test_rule", config.ProcessingRules[0].Name)
				assert.Equal(t, `^\d{4}`, config.ProcessingRules[0].Pattern)
			},
		},
		{
			name: "Test comprehensive YAML config when JSON fails",
			data: []byte(`
logs:
  - type: journald
    path: /var/log/journal/
    source: comprehensive_source
    service: comprehensive_service
    tags: comprehensive_tag1, comprehensive_tag2
    encoding: utf-16-be
    port: 9090
    container_mode: false
    include_units: systemd-unit.service
    exclude_units: datadog-agent.service
    log_processing_rules:
    - type: mask_sequences
      name: mask_test
      pattern: (password|secret)
      replace_placeholder: "[MASKED]"
`),
			assert: func(t *testing.T, configs []*LogsConfig, err error) {
				assert.Nil(t, err)
				require.Equal(t, len(configs), 1)
				config := configs[0]
				assert.Equal(t, "journald", config.Type)
				assert.Equal(t, "/var/log/journal/", config.Path)
				assert.Equal(t, "comprehensive_source", config.Source)
				assert.Equal(t, "comprehensive_service", config.Service)
				require.Equal(t, len(config.Tags), 2)
				assert.Equal(t, "comprehensive_tag1", config.Tags[0])
				assert.Equal(t, " comprehensive_tag2", config.Tags[1])
				assert.Equal(t, "utf-16-be", config.Encoding)
				assert.Equal(t, 9090, config.Port)
				assert.Equal(t, false, config.ContainerMode)
				require.Equal(t, len(config.IncludeSystemUnits), 1)
				assert.Equal(t, "systemd-unit.service", config.IncludeSystemUnits[0])
				require.Equal(t, len(config.ExcludeSystemUnits), 1)
				assert.Equal(t, "datadog-agent.service", config.ExcludeSystemUnits[0])
				require.Equal(t, len(config.ProcessingRules), 1)
				assert.Equal(t, "mask_sequences", config.ProcessingRules[0].Type)
				assert.Equal(t, "mask_test", config.ProcessingRules[0].Name)
				assert.Equal(t, "(password|secret)", config.ProcessingRules[0].Pattern)
				assert.Equal(t, "[MASKED]", config.ProcessingRules[0].ReplacePlaceholder)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configs, err := ParseJSONOrYAML(tt.data)
			tt.assert(t, configs, err)
		})
	}
}
