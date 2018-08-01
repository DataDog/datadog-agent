// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

const testsPath = "tests"

func TestAvailableIntegrationConfigs(t *testing.T) {
	ddconfdPath := filepath.Join(testsPath, "complete", "conf.d")
	assert.Equal(t, []string{"integration.yaml", "integration2.yml", "integration5.yml", "integration6.yml", "misconfigured_integration.yaml", "integration.d/integration3.yaml", "integration.d/integration4.yaml"}, availableIntegrationConfigs(ddconfdPath))
}

func TestBuildLogsAgentIntegrationsConfigs(t *testing.T) {
	ddconfdPath := filepath.Join(testsPath, "complete", "conf.d")

	allSources, err := buildLogSources(ddconfdPath, false, -1)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(allSources.GetSources())-len(allSources.GetValidSources()))

	sources := allSources.GetValidSources()

	assert.Equal(t, "file", sources[0].Config.Type)
	assert.Equal(t, "/var/log/access.log", sources[0].Config.Path)
	assert.Equal(t, "nginx", sources[0].Config.Service)
	assert.Equal(t, "nginx", sources[0].Config.Source)
	assert.Equal(t, "http_access", sources[0].Config.SourceCategory)
	assert.Equal(t, []string{"env:prod"}, sources[0].Config.Tags)

	assert.Equal(t, "tcp", sources[1].Config.Type)
	assert.Equal(t, 10514, sources[1].Config.Port)
	assert.Equal(t, "", sources[1].Config.Service)
	assert.Equal(t, "", sources[1].Config.Source)
	assert.Equal(t, 0, len(sources[1].Config.Tags))

	assert.Equal(t, "journald", sources[2].Config.Type)

	assert.Equal(t, "windows_event", sources[3].Config.Type)
	assert.Equal(t, "System", sources[3].Config.ChannelPath)
	assert.Equal(t, "*", sources[3].Config.Query)

	assert.Equal(t, "docker", sources[4].Config.Type)
	assert.Equal(t, "test", sources[4].Config.Image)

	assert.Equal(t, []string{"env:prod", "foo:bar"}, sources[5].Config.Tags)
	assert.Equal(t, []string{"env:prod", "foo:bar"}, sources[6].Config.Tags)
	assert.Equal(t, []string{"env:prod", "foo:bar"}, sources[7].Config.Tags)

	// processing
	assert.Equal(t, 0, len(sources[0].Config.ProcessingRules))
	assert.Equal(t, 4, len(sources[1].Config.ProcessingRules))

	pRule := sources[1].Config.ProcessingRules[0]
	assert.Equal(t, "mask_sequences", pRule.Type)
	assert.Equal(t, "mocked_mask_rule", pRule.Name)
	assert.Equal(t, "[mocked]", pRule.ReplacePlaceholder)
	assert.Equal(t, []byte("[mocked]"), pRule.ReplacePlaceholderBytes)
	assert.Equal(t, ".*", pRule.Pattern)
	re := pRule.Reg
	assert.True(t, re.MatchString("123"))

	mRule := sources[1].Config.ProcessingRules[1]
	assert.Equal(t, "multi_line", mRule.Type)
	assert.Equal(t, "numbers", mRule.Name)
	re = mRule.Reg
	assert.True(t, re.MatchString("123"))
	assert.False(t, re.MatchString("a123"))

	eRule := sources[1].Config.ProcessingRules[2]
	assert.Equal(t, "exclude_at_match", eRule.Type)
	assert.Equal(t, "exclude_bob", eRule.Name)
	assert.Equal(t, "^bob", eRule.Pattern)
	re = eRule.Reg
	assert.True(t, re.MatchString("boba"))
	assert.False(t, re.MatchString("abob"))

	iRule := sources[1].Config.ProcessingRules[3]
	assert.Equal(t, "include_at_match", iRule.Type)
	assert.Equal(t, "include_datadoghq", iRule.Name)
	assert.Equal(t, ".*@datadoghq.com$", iRule.Pattern)
	re = iRule.Reg
	assert.True(t, re.MatchString("bob@datadoghq.com"))
	assert.False(t, re.MatchString("bob"))
}

func TestCompileProcessingRules(t *testing.T) {
	var config *LogsConfig
	var rules []LogsProcessingRule
	var err error

	rules = []LogsProcessingRule{{Pattern: "(?=abf)", Type: IncludeAtMatch}}
	config = &LogsConfig{ProcessingRules: rules}
	err = Compile(config)
	assert.NotNil(t, err)
	assert.Nil(t, rules[0].Reg)

	rules = []LogsProcessingRule{{Pattern: "[[:alnum:]]{5}", Type: IncludeAtMatch}}
	config = &LogsConfig{ProcessingRules: rules}
	err = Compile(config)
	assert.Nil(t, err)
	assert.NotNil(t, rules[0].Reg)
	assert.True(t, rules[0].Reg.MatchString("abcde"))

	rules = []LogsProcessingRule{{Pattern: "[[:alnum:]]{5}"}}
	config = &LogsConfig{ProcessingRules: rules}
	err = Compile(config)
	assert.NotNil(t, err)
	assert.Nil(t, rules[0].Reg)

	rules = []LogsProcessingRule{{Pattern: "", Type: IncludeAtMatch}}
	config = &LogsConfig{ProcessingRules: rules}
	err = Compile(config)
	assert.NotNil(t, err)
	assert.Nil(t, rules[0].Reg)
}

func TestBuildLogsAgentIntegrationConfigsWithMisconfiguredFile(t *testing.T) {
	var ddconfdPath string
	var err error

	ddconfdPath = filepath.Join(testsPath, "misconfigured_1")
	_, err = buildLogSources(ddconfdPath, false, -1)
	assert.NotNil(t, err)

	ddconfdPath = filepath.Join(testsPath, "misconfigured_2", "conf.d")
	_, err = buildLogSources(ddconfdPath, false, -1)
	assert.NotNil(t, err)

	ddconfdPath = filepath.Join(testsPath, "misconfigured_3", "conf.d")
	_, err = buildLogSources(ddconfdPath, false, -1)
	assert.NotNil(t, err)

	ddconfdPath = filepath.Join(testsPath, "misconfigured_4", "conf.d")
	_, err = buildLogSources(ddconfdPath, false, -1)
	assert.NotNil(t, err)
}

func TestIntegrationName(t *testing.T) {
	var integrationName string
	var err error

	integrationName, err = buildIntegrationName("foo.d/bar.yml")
	assert.Equal(t, "foo", integrationName)
	assert.Nil(t, err)

	integrationName, err = buildIntegrationName("bar.yaml")
	assert.Equal(t, "bar", integrationName)
	assert.Nil(t, err)

	integrationName, err = buildIntegrationName("bar.yml")
	assert.Equal(t, "bar", integrationName)
	assert.Nil(t, err)

	_, err = buildIntegrationName("foo.bar")
	assert.NotNil(t, err)

	_, err = buildIntegrationName("foo.b/bar.yml")
	assert.NotNil(t, err)
}
