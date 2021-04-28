// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"github.com/stretchr/testify/suite"
)

type YamlConfigTestSuite struct {
	suite.Suite
	config *coreConfig.MockConfig
}

func (suite *YamlConfigTestSuite) SetupTest() {
	suite.config = coreConfig.Mock()
}

func (suite *YamlConfigTestSuite) TestExtractOrchestratorDDOrchestratorUrl() {
	suite.config.Set("api_key", "wassupkey")
	suite.config.Set("orchestrator_explorer.orchestrator_dd_url", "https://orchestrator-link.com")
	actual, err := extractOrchestratorDDUrl()
	suite.NoError(err)
	expected, err := url.Parse("https://orchestrator-link.com")
	suite.NoError(err)
	suite.Equal(expected, actual)
}

func (suite *YamlConfigTestSuite) TestExtractOrchestratorDDProcessUrl() {
	suite.config.Set("api_key", "wassupkey")
	suite.config.Set("process_config.orchestrator_dd_url", "https://process-link.com")
	actual, err := extractOrchestratorDDUrl()
	suite.NoError(err)
	expected, err := url.Parse("https://process-link.com")
	suite.NoError(err)
	suite.Equal(expected, actual)
}

func (suite *YamlConfigTestSuite) TestExtractOrchestratorDDNonSet() {
	actual, err := extractOrchestratorDDUrl()
	suite.NoError(err)
	expected, err := url.Parse("https://orchestrator.datadoghq.com")
	suite.NoError(err)
	suite.Equal(expected, actual)
}

func (suite *YamlConfigTestSuite) TestExtractOrchestratorPrecedence() {
	suite.config.Set("api_key", "wassupkey")
	suite.config.Set("process_config.orchestrator_dd_url", "https://process-link.com")
	suite.config.Set("orchestrator_explorer.orchestrator_dd_url", "https://orchestrator-link.com")
	actual, err := extractOrchestratorDDUrl()
	suite.NoError(err)
	expected, err := url.Parse("https://orchestrator-link.com")
	suite.NoError(err)
	suite.Equal(expected, actual)
}

func (suite *YamlConfigTestSuite) TestExtractOrchestratorProcessEndpoints() {
	expected := make(map[string]string)
	expected["key1"] = "process1.com"
	expected["key2"] = "process2.com"
	expected["key3"] = "process2.com"
	expected["apikey_20"] = "orchestrator.datadoghq.com"
	var actualEndpoints []apicfg.Endpoint

	suite.config.Set("api_key", "wassupkey")
	suite.config.Set("process_config.orchestrator_additional_endpoints", `{"https://process1.com": ["key1"], "https://process2.com": ["key2", "key3"]}`)
	err := extractOrchestratorAdditionalEndpoints(&url.URL{}, &actualEndpoints)
	suite.NoError(err)
	for _, actual := range actualEndpoints {
		suite.Equal(expected[actual.APIKey], actual.Endpoint.Hostname())
	}
}

func (suite *YamlConfigTestSuite) TestExtractOrchestratorOrchestratorEndpoints() {
	expected := make(map[string]string)
	expected["key1"] = "orchestrator1.com"
	expected["key2"] = "orchestrator2.com"
	expected["key3"] = "orchestrator2.com"
	expected["apikey_20"] = "orchestrator.datadoghq.com"
	var actualEndpoints []apicfg.Endpoint

	suite.config.Set("api_key", "wassupkey")
	suite.config.Set("orchestrator_explorer.orchestrator_additional_endpoints", `{"https://orchestrator1.com": ["key1"], "https://orchestrator2.com": ["key2", "key3"]}`)
	err := extractOrchestratorAdditionalEndpoints(&url.URL{}, &actualEndpoints)
	suite.NoError(err)
	for _, actual := range actualEndpoints {
		suite.Equal(expected[actual.APIKey], actual.Endpoint.Hostname())
	}
}

func (suite *YamlConfigTestSuite) TestExtractOrchestratorEndpointsPrecedence() {
	expected := make(map[string]string)
	expected["key1"] = "orchestrator1.com"
	expected["key2"] = "orchestrator2.com"
	expected["key3"] = "orchestrator2.com"
	expected["apikey_20"] = "orchestrator.datadoghq.com"
	// verifying that we do not overwrite an existing endpoint.
	expected["test"] = "test.com"

	u, _ := url.Parse("https://test.com")
	actualEndpoints := []apicfg.Endpoint{{APIKey: "test", Endpoint: u}}

	suite.config.Set("api_key", "wassupkey")
	suite.config.Set("process_config.orchestrator_additional_endpoints", `{"https://process1.com": ["key1"], "https://process2.com": ["key2", "key3"]}`)
	suite.config.Set("orchestrator_explorer.orchestrator_additional_endpoints", `{"https://orchestrator1.com": ["key1"], "https://orchestrator2.com": ["key2", "key3"]}`)
	err := extractOrchestratorAdditionalEndpoints(&url.URL{}, &actualEndpoints)
	suite.NoError(err)
	for _, actual := range actualEndpoints {
		suite.Equal(expected[actual.APIKey], actual.Endpoint.Hostname())
	}
}

func (suite *YamlConfigTestSuite) TestEnvConfigDDURL() {
	ddOrchestratorURL := "DD_ORCHESTRATOR_EXPLORER_ORCHESTRATOR_DD_URL"
	expectedValue := "123.datadoghq.com"
	os.Setenv(ddOrchestratorURL, expectedValue)
	defer os.Unsetenv(ddOrchestratorURL)

	orchestratorCfg := NewDefaultOrchestratorConfig()
	err := orchestratorCfg.Load()
	suite.NoError(err)

	suite.Equal(expectedValue, orchestratorCfg.OrchestratorEndpoints[0].Endpoint.Path)
}

func (suite *YamlConfigTestSuite) TestEnvConfigMessageSize() {
	ddMaxMessage := "DD_ORCHESTRATOR_EXPLORER_MAX_PER_MESSAGE"
	expectedValue := "50"
	os.Setenv(ddMaxMessage, expectedValue)
	defer os.Unsetenv(ddMaxMessage)

	orchestratorCfg := NewDefaultOrchestratorConfig()
	err := orchestratorCfg.Load()
	suite.NoError(err)

	i, err := strconv.Atoi(expectedValue)
	suite.NoError(err)
	suite.Equal(i, orchestratorCfg.MaxPerMessage)
}

func (suite *YamlConfigTestSuite) TestEnvConfigMessageSizeTooHigh() {
	ddMaxMessage := "DD_ORCHESTRATOR_EXPLORER_MAX_PER_MESSAGE"
	expectedDefaultValue := 100

	os.Setenv(ddMaxMessage, "150")
	defer os.Unsetenv(ddMaxMessage)

	orchestratorCfg := NewDefaultOrchestratorConfig()
	err := orchestratorCfg.Load()
	suite.NoError(err)

	suite.Equal(expectedDefaultValue, orchestratorCfg.MaxPerMessage)
}

func (suite *YamlConfigTestSuite) TestEnvConfigSensitiveWords() {
	ddSensitiveWords := "DD_ORCHESTRATOR_EXPLORER_CUSTOM_SENSITIVE_WORDS"
	expectedValue := "token consul"
	os.Setenv(ddSensitiveWords, expectedValue)
	defer os.Unsetenv(ddSensitiveWords)

	orchestratorCfg := NewDefaultOrchestratorConfig()
	err := orchestratorCfg.Load()
	suite.NoError(err)

	for _, val := range strings.Split(expectedValue, " ") {
		suite.Contains(orchestratorCfg.Scrubber.LiteralSensitivePatterns, val)
	}
}

func (suite *YamlConfigTestSuite) TestNoEnvConfigArgsScrubbing() {

	orchestratorCfg := NewDefaultOrchestratorConfig()
	err := orchestratorCfg.Load()
	suite.NoError(err)

	cases := []struct {
		cmdline       []string
		parsedCmdline []string
	}{
		{
			[]string{"spidly", "--token=123", "consul", "123", "--dd_api_key=1234"},
			[]string{"spidly", "--token=123", "consul", "123", "--dd_api_key=********"},
		},
	}

	for i := range cases {
		actual, _ := orchestratorCfg.Scrubber.ScrubSimpleCommand(cases[i].cmdline)
		suite.Equal(cases[i].parsedCmdline, actual)
	}
}

func (suite *YamlConfigTestSuite) TestOnlyEnvConfigArgsScrubbing() {

	suite.config.Set("orchestrator_explorer.custom_sensitive_words", `["token","consul"]`)

	orchestratorCfg := NewDefaultOrchestratorConfig()
	err := orchestratorCfg.Load()
	suite.NoError(err)

	cases := []struct {
		cmdline       []string
		parsedCmdline []string
	}{
		{
			[]string{"spidly", "--gitlab_token=123", "consul_thing", "123", "--dd_api_key=1234"},
			[]string{"spidly", "--gitlab_token=********", "consul_thing", "********", "--dd_api_key=********"},
		},
	}

	for i := range cases {
		actual, _ := orchestratorCfg.Scrubber.ScrubSimpleCommand(cases[i].cmdline)
		suite.Equal(cases[i].parsedCmdline, actual)
	}
}

func (suite *YamlConfigTestSuite) TestOnlyEnvContainsConfigArgsScrubbing() {

	suite.config.Set("orchestrator_explorer.custom_sensitive_words", `["token","consul"]`)

	orchestratorCfg := NewDefaultOrchestratorConfig()
	err := orchestratorCfg.Load()
	suite.NoError(err)

	cases := []struct {
		word     string
		expected bool
	}{
		{
			"spidly",
			false,
		},
		{
			"gitlab_token",
			true,
		},
		{
			"GITLAB_TOKEn",
			true,
		},
		{
			"consul_word",
			true,
		},
	}

	for i := range cases {
		actual := orchestratorCfg.Scrubber.ContainsSensitiveWord(cases[i].word)
		suite.Equal(cases[i].expected, actual)
	}
}

func TestYamlConfigTestSuite(t *testing.T) {
	suite.Run(t, new(YamlConfigTestSuite))
}
