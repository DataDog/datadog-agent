// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"strings"
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
	assert.Equal(t, []string{"a", "b:d"}, config.Tags)

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

func TestParseYamlWithValidFormatShouldSucceed(t *testing.T) {
	data := []byte(`
logs:
  - type: file
    path: /var/log/app.log
    tags: a, b:c
  - type: udp
    source: foo
    service: bar
  - type: docker
    log_processing_rules:
      - type: include_at_match
        name: numbers
        pattern: ^[0-9]+$
`)

	configs, err := ParseYaml(data)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(configs))

	var config *LogsConfig
	var tag string
	var rule ProcessingRule

	config = configs[0]
	assert.Equal(t, FileType, config.Type)
	assert.Equal(t, "/var/log/app.log", config.Path)
	assert.Equal(t, 2, len(config.Tags))

	tag = config.Tags[0]
	assert.Equal(t, "a", strings.TrimSpace(tag))

	tag = config.Tags[1]
	assert.Equal(t, "b:c", strings.TrimSpace(tag))

	config = configs[1]
	assert.Equal(t, UDPType, config.Type)
	assert.Equal(t, "foo", config.Source)
	assert.Equal(t, "bar", config.Service)

	config = configs[2]
	assert.Equal(t, DockerType, config.Type)
	assert.Equal(t, 1, len(config.ProcessingRules))

	rule = config.ProcessingRules[0]
	assert.Equal(t, IncludeAtMatch, rule.Type)
	assert.Equal(t, "numbers", rule.Name)
	assert.Equal(t, "^[0-9]+$", rule.Pattern)
}

func TestParseYamlWithInvalidFormatShouldFail(t *testing.T) {
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
		configs, _ := ParseYaml([]byte(format))
		assert.Equal(t, 0, len(configs))
	}
}
