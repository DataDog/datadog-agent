// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package config

import (
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

const testsPath = "tests"

func TestAvailableIntegrationConfigs(t *testing.T) {
	ddconfdPath := filepath.Join(testsPath, "complete", "conf.d")
	assert.Equal(t, []string{"integration.yaml", "integration2.yml", "misconfigured_integration.yaml", "integration.d/integration3.yaml"}, availableIntegrationConfigs(ddconfdPath))
}

func TestBuildLogsAgentIntegrationsConfigs(t *testing.T) {
	ddconfdPath := filepath.Join(testsPath, "complete", "conf.d")
	var testConfig = viper.New()
	buildLogsAgentIntegrationsConfig(testConfig, ddconfdPath)

	rules := getLogsSources(testConfig)
	assert.Equal(t, 3, len(rules))
	assert.Equal(t, "file", rules[0].Type)
	assert.Equal(t, "/var/log/access.log", rules[0].Path)
	assert.Equal(t, "nginx", rules[0].Service)
	assert.Equal(t, "nginx", rules[0].Source)
	assert.Equal(t, "http_access", rules[0].SourceCategory)
	assert.Equal(t, "", rules[0].Logset)
	assert.Equal(t, "env:prod", rules[0].Tags)
	assert.Equal(t, "[dd ddsource=\"nginx\"][dd ddsourcecategory=\"http_access\"][dd ddtags=\"env:prod\"]", string(rules[0].TagsPayload))

	assert.Equal(t, "tcp", rules[1].Type)
	assert.Equal(t, 10514, rules[1].Port)
	assert.Equal(t, "devteam", rules[1].Logset)
	assert.Equal(t, "", rules[1].Service)
	assert.Equal(t, "", rules[1].Source)
	assert.Equal(t, 0, len(rules[1].Tags))

	assert.Equal(t, "docker", rules[2].Type)
	assert.Equal(t, "test", rules[2].Image)

	// processing
	assert.Equal(t, 0, len(rules[0].ProcessingRules))
	assert.Equal(t, 4, len(rules[1].ProcessingRules))

	pRule := rules[1].ProcessingRules[0]
	assert.Equal(t, "mask_sequences", pRule.Type)
	assert.Equal(t, "mocked_mask_rule", pRule.Name)
	assert.Equal(t, "[mocked]", pRule.ReplacePlaceholder)
	assert.Equal(t, []byte("[mocked]"), pRule.ReplacePlaceholderBytes)
	assert.Equal(t, ".*", pRule.Pattern)

	mRule := rules[1].ProcessingRules[1]
	assert.Equal(t, "multi_line", mRule.Type)
	assert.Equal(t, "numbers", mRule.Name)
	re := mRule.Reg
	assert.True(t, re.MatchString("123"))
	assert.False(t, re.MatchString("a123"))

	eRule := rules[1].ProcessingRules[2]
	assert.Equal(t, "exclude_at_match", eRule.Type)
	assert.Equal(t, "exclude_bob", eRule.Name)
	assert.Equal(t, "^bob", eRule.Pattern)

	iRule := rules[1].ProcessingRules[3]
	assert.Equal(t, "include_at_match", iRule.Type)
	assert.Equal(t, "include_datadoghq", iRule.Name)
	assert.Equal(t, ".*@datadoghq.com$", iRule.Pattern)
}

func TestBuildLogsAgentIntegrationConfigsWithMisconfiguredFile(t *testing.T) {
	var testConfig = viper.New()
	var ddconfdPath string
	var err error
	ddconfdPath = filepath.Join(testsPath, "misconfigured_1")
	err = buildLogsAgentIntegrationsConfig(testConfig, ddconfdPath)
	assert.NotNil(t, err)

	ddconfdPath = filepath.Join(testsPath, "misconfigured_2", "conf.d")
	err = buildLogsAgentIntegrationsConfig(testConfig, ddconfdPath)
	assert.NotNil(t, err)

	ddconfdPath = filepath.Join(testsPath, "misconfigured_3", "conf.d")
	err = buildLogsAgentIntegrationsConfig(testConfig, ddconfdPath)
	assert.NotNil(t, err)

	ddconfdPath = filepath.Join(testsPath, "misconfigured_4", "conf.d")
	err = buildLogsAgentIntegrationsConfig(testConfig, ddconfdPath)
	assert.NotNil(t, err)

	ddconfdPath = filepath.Join(testsPath, "misconfigured_5", "conf.d")
	err = buildLogsAgentIntegrationsConfig(testConfig, ddconfdPath)
	assert.NotNil(t, err)
}

func TestBuildTagsPayload(t *testing.T) {
	assert.Equal(t, "-", string(BuildTagsPayload("", "", "")))
	assert.Equal(t, "[dd ddtags=\"hello:world\"]", string(BuildTagsPayload("hello:world", "", "")))
	assert.Equal(t, "[dd ddsource=\"nginx\"][dd ddsourcecategory=\"http_access\"][dd ddtags=\"hello:world, hi\"]", string(BuildTagsPayload("hello:world, hi", "nginx", "http_access")))
}

func TestLogsAgentDefaultValues(t *testing.T) {
	assert.Equal(t, "", LogsAgent.GetString("logset"))
	assert.Equal(t, "intake.logs.datadoghq.com", LogsAgent.GetString("log_dd_url"))
	assert.Equal(t, 10516, LogsAgent.GetInt("log_dd_port"))
	assert.Equal(t, false, LogsAgent.GetBool("skip_ssl_validation"))
	assert.Equal(t, false, LogsAgent.GetBool("dev_mode_no_ssl"))
	assert.Equal(t, false, LogsAgent.GetBool("log_enabled"))
	assert.Equal(t, 100, LogsAgent.GetInt("log_open_files_limit"))
}

func TestIntegrationsStatus(t *testing.T) {
	var testConfig = viper.New()
	var ddconfdPath string
	var integrationsStatus []status.Integration

	ddconfdPath = filepath.Join(testsPath, "complete", "conf.d")
	buildLogsAgentIntegrationsConfig(testConfig, ddconfdPath)
	integrationsStatus = getIntegrationsStatus(testConfig)
	assert.Equal(t, 4, len(integrationsStatus))

	var integration status.Integration

	ddconfdPath = filepath.Join(testsPath, "misconfigured_1")
	buildLogsAgentIntegrationsConfig(testConfig, ddconfdPath)
	integrationsStatus = getIntegrationsStatus(testConfig)
	assert.Equal(t, 1, len(integrationsStatus))
	integration = integrationsStatus[0]
	assert.Equal(t, "integration", integration.Name)
	assert.Equal(t, 1, len(integration.Errors))

	ddconfdPath = filepath.Join(testsPath, "misconfigured_2", "conf.d")
	buildLogsAgentIntegrationsConfig(testConfig, ddconfdPath)
	integrationsStatus = getIntegrationsStatus(testConfig)
	assert.Equal(t, 1, len(integrationsStatus))
	integration = integrationsStatus[0]
	assert.Equal(t, "integration", integration.Name)
	assert.Equal(t, 1, len(integration.Errors))

	ddconfdPath = filepath.Join(testsPath, "misconfigured_3", "conf.d")
	buildLogsAgentIntegrationsConfig(testConfig, ddconfdPath)
	integrationsStatus = getIntegrationsStatus(testConfig)
	assert.Equal(t, 1, len(integrationsStatus))
	integration = integrationsStatus[0]
	assert.Equal(t, "integration", integration.Name)
	assert.Equal(t, 1, len(integration.Errors))

	ddconfdPath = filepath.Join(testsPath, "misconfigured_4", "conf.d")
	buildLogsAgentIntegrationsConfig(testConfig, ddconfdPath)
	integrationsStatus = getIntegrationsStatus(testConfig)
	assert.Equal(t, 1, len(integrationsStatus))
	integration = integrationsStatus[0]
	assert.Equal(t, "integration", integration.Name)
	assert.Equal(t, 1, len(integration.Errors))

	ddconfdPath = filepath.Join(testsPath, "misconfigured_5", "conf.d")
	buildLogsAgentIntegrationsConfig(testConfig, ddconfdPath)
	integrationsStatus = getIntegrationsStatus(testConfig)
	assert.Equal(t, 1, len(integrationsStatus))
	integration = integrationsStatus[0]
	assert.Equal(t, "integration", integration.Name)
	assert.Equal(t, 1, len(integration.Errors))

	ddconfdPath = filepath.Join(testsPath, "does_not_exist")
	buildLogsAgentIntegrationsConfig(testConfig, ddconfdPath)
	integrationsStatus = getIntegrationsStatus(testConfig)
	assert.Equal(t, 0, len(integrationsStatus))
}
