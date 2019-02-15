// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/suite"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
)

type ConfigTestSuite struct {
	suite.Suite
	config *coreConfig.MockConfig
}

func (suite *ConfigTestSuite) SetupTest() {
	suite.config = coreConfig.Mock()
}

func (suite *ConfigTestSuite) TestDefaultDatadogConfig() {
	suite.Equal(false, suite.config.GetBool("log_enabled"))
	suite.Equal(false, suite.config.GetBool("logs_enabled"))
	suite.Equal("", suite.config.GetString("logset"))
	suite.Equal("", suite.config.GetString("logs_config.dd_url"))
	suite.Equal(10516, suite.config.GetInt("logs_config.dd_port"))
	suite.Equal(false, suite.config.GetBool("logs_config.dev_mode_no_ssl"))
	suite.Equal("agent-443-intake.logs.datadoghq.com", suite.config.GetString("logs_config.dd_url_443"))
	suite.Equal(false, suite.config.GetBool("logs_config.use_port_443"))
	suite.Equal(true, suite.config.GetBool("logs_config.dev_mode_use_proto"))
	suite.Equal(100, suite.config.GetInt("logs_config.open_files_limit"))
	suite.Equal(9000, suite.config.GetInt("logs_config.frame_size"))
	suite.Equal("", suite.config.GetString("logs_config.socks5_proxy_address"))
	suite.Equal("", suite.config.GetString("logs_config.logs_dd_url"))
	suite.Equal(false, suite.config.GetBool("logs_config.logs_no_ssl"))
	suite.Equal(30, suite.config.GetInt("logs_config.stop_grace_period"))
}

func (suite *ConfigTestSuite) TestDefaultSources() {
	var sources []*LogSource
	var source *LogSource

	suite.config.Set("logs_config.container_collect_all", true)

	sources = DefaultSources()
	suite.Equal(1, len(sources))

	source = sources[0]
	suite.Equal("container_collect_all", source.Name)
	suite.Equal(DockerType, source.Config.Type)
	suite.Equal("docker", source.Config.Source)
	suite.Equal("docker", source.Config.Service)
}

func (suite *ConfigTestSuite) TestGlobalProcessingRules() {
	var (
		rules []*ProcessingRule
		rule  *ProcessingRule
		err   error
	)

	suite.config.Set("logs_config.processing_rules", nil)

	rules, err = GlobalProcessingRules()
	suite.Nil(err)
	suite.Equal(0, len(rules))

	suite.config.Set("logs_config.processing_rules", []map[string]interface{}{
		{
			"type":    "exclude_at_match",
			"name":    "exclude_foo",
			"pattern": "foo",
		},
	})

	rules, err = GlobalProcessingRules()
	suite.Nil(err)
	suite.Equal(1, len(rules))

	rule = rules[0]
	suite.Equal(ExcludeAtMatch, rule.Type)
	suite.Equal("exclude_foo", rule.Name)
	suite.Equal("foo", rule.Pattern)
	suite.NotNil(rule.Regex)
}

func TestConfigTestSuite(t *testing.T) {
	suite.Run(t, new(ConfigTestSuite))
}
