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
	assert.Equal(t, []string{"integration.yaml", "integration2.yml", "misconfigured_integration.yaml", "integration.d/integration3.yaml"}, availableIntegrationConfigs(ddconfdPath))
}

func TestBuildLogsAgentIntegrationsConfigs(t *testing.T) {
	ddconfdPath := filepath.Join(testsPath, "complete", "conf.d")
	sources, sourcesToTrack, err := buildLogsSources(ddconfdPath)

	assert.Nil(t, err)
	assert.Equal(t, 3, len(sources))
	assert.Equal(t, 4, len(sourcesToTrack))

	assert.Equal(t, "file", sources[0].Type)
	assert.Equal(t, "/var/log/access.log", sources[0].Path)
	assert.Equal(t, "nginx", sources[0].Service)
	assert.Equal(t, "nginx", sources[0].Source)
	assert.Equal(t, "http_access", sources[0].SourceCategory)
	assert.Equal(t, "", sources[0].Logset)
	assert.Equal(t, "env:prod", sources[0].Tags)
	assert.Equal(t, "[dd ddsource=\"nginx\"][dd ddsourcecategory=\"http_access\"][dd ddtags=\"env:prod\"]", string(sources[0].TagsPayload))

	assert.Equal(t, "tcp", sources[1].Type)
	assert.Equal(t, 10514, sources[1].Port)
	assert.Equal(t, "devteam", sources[1].Logset)
	assert.Equal(t, "", sources[1].Service)
	assert.Equal(t, "", sources[1].Source)
	assert.Equal(t, 0, len(sources[1].Tags))

	assert.Equal(t, "docker", sources[2].Type)
	assert.Equal(t, "test", sources[2].Image)

	// processing
	assert.Equal(t, 0, len(sources[0].ProcessingRules))
	assert.Equal(t, 4, len(sources[1].ProcessingRules))

	pRule := sources[1].ProcessingRules[0]
	assert.Equal(t, "mask_sequences", pRule.Type)
	assert.Equal(t, "mocked_mask_rule", pRule.Name)
	assert.Equal(t, "[mocked]", pRule.ReplacePlaceholder)
	assert.Equal(t, []byte("[mocked]"), pRule.ReplacePlaceholderBytes)
	assert.Equal(t, ".*", pRule.Pattern)

	mRule := sources[1].ProcessingRules[1]
	assert.Equal(t, "multi_line", mRule.Type)
	assert.Equal(t, "numbers", mRule.Name)
	re := mRule.Reg
	assert.True(t, re.MatchString("123"))
	assert.False(t, re.MatchString("a123"))

	eRule := sources[1].ProcessingRules[2]
	assert.Equal(t, "exclude_at_match", eRule.Type)
	assert.Equal(t, "exclude_bob", eRule.Name)
	assert.Equal(t, "^bob", eRule.Pattern)

	iRule := sources[1].ProcessingRules[3]
	assert.Equal(t, "include_at_match", iRule.Type)
	assert.Equal(t, "include_datadoghq", iRule.Name)
	assert.Equal(t, ".*@datadoghq.com$", iRule.Pattern)
}

func TestBuildLogsAgentIntegrationConfigsWithMisconfiguredFile(t *testing.T) {
	var ddconfdPath string
	var err error
	ddconfdPath = filepath.Join(testsPath, "misconfigured_1")
	_, _, err = buildLogsSources(ddconfdPath)
	assert.NotNil(t, err)

	ddconfdPath = filepath.Join(testsPath, "misconfigured_2", "conf.d")
	_, _, err = buildLogsSources(ddconfdPath)
	assert.NotNil(t, err)

	ddconfdPath = filepath.Join(testsPath, "misconfigured_3", "conf.d")
	_, _, err = buildLogsSources(ddconfdPath)
	assert.NotNil(t, err)

	ddconfdPath = filepath.Join(testsPath, "misconfigured_4", "conf.d")
	_, _, err = buildLogsSources(ddconfdPath)
	assert.NotNil(t, err)

	ddconfdPath = filepath.Join(testsPath, "misconfigured_5", "conf.d")
	_, _, err = buildLogsSources(ddconfdPath)
	assert.NotNil(t, err)
}

func TestBuildTagsPayload(t *testing.T) {
	assert.Equal(t, "-", string(BuildTagsPayload("", "", "")))
	assert.Equal(t, "[dd ddtags=\"hello:world\"]", string(BuildTagsPayload("hello:world", "", "")))
	assert.Equal(t, "[dd ddsource=\"nginx\"][dd ddsourcecategory=\"http_access\"][dd ddtags=\"hello:world, hi\"]", string(BuildTagsPayload("hello:world, hi", "nginx", "http_access")))
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
