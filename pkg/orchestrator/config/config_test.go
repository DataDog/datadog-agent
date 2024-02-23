// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
)

type YamlConfigTestSuite struct {
	suite.Suite
	config *coreConfig.MockConfig
}

func (suite *YamlConfigTestSuite) SetupTest() {
	suite.config = coreConfig.Mock(nil)
}

func (suite *YamlConfigTestSuite) TestExtractOrchestratorDDOrchestratorUrl() {
	suite.config.SetWithoutSource("api_key", "wassupkey")
	suite.config.SetWithoutSource("orchestrator_explorer.orchestrator_dd_url", "https://orchestrator-link.com")
	actual, err := extractOrchestratorDDUrl()
	suite.NoError(err)
	expected, err := url.Parse("https://orchestrator-link.com")
	suite.NoError(err)
	suite.Equal(expected, actual)
}

func (suite *YamlConfigTestSuite) TestExtractOrchestratorDDProcessUrl() {
	suite.config.SetWithoutSource("api_key", "wassupkey")
	suite.config.SetWithoutSource("process_config.orchestrator_dd_url", "https://process-link.com")
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
	suite.config.SetWithoutSource("api_key", "wassupkey")
	suite.config.SetWithoutSource("process_config.orchestrator_dd_url", "https://process-link.com")
	suite.config.SetWithoutSource("orchestrator_explorer.orchestrator_dd_url", "https://orchestrator-link.com")
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

	suite.config.SetWithoutSource("api_key", "wassupkey")
	suite.config.SetWithoutSource("process_config.orchestrator_additional_endpoints", `{"https://process1.com": ["key1"], "https://process2.com": ["key2", "key3"]}`)
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

	suite.config.SetWithoutSource("api_key", "wassupkey")
	suite.config.SetWithoutSource("orchestrator_explorer.orchestrator_additional_endpoints", `{"https://orchestrator1.com": ["key1"], "https://orchestrator2.com": ["key2", "key3"]}`)
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

	suite.config.SetWithoutSource("api_key", "wassupkey")
	suite.config.SetWithoutSource("process_config.orchestrator_additional_endpoints", `{"https://process1.com": ["key1"], "https://process2.com": ["key2", "key3"]}`)
	suite.config.SetWithoutSource("orchestrator_explorer.orchestrator_additional_endpoints", `{"https://orchestrator1.com": ["key1"], "https://orchestrator2.com": ["key2", "key3"]}`)
	err := extractOrchestratorAdditionalEndpoints(&url.URL{}, &actualEndpoints)
	suite.NoError(err)
	for _, actual := range actualEndpoints {
		suite.Equal(expected[actual.APIKey], actual.Endpoint.Hostname())
	}
}

func (suite *YamlConfigTestSuite) TestEnvConfigDDURL() {
	ddOrchestratorURL := "DD_ORCHESTRATOR_URL"
	expectedValue := "123.datadoghq.com"
	suite.T().Setenv(ddOrchestratorURL, expectedValue)

	orchestratorCfg := NewDefaultOrchestratorConfig()
	err := orchestratorCfg.Load()
	suite.NoError(err)

	suite.Equal(expectedValue, orchestratorCfg.OrchestratorEndpoints[0].Endpoint.Path)

	// Override to make sure the precedence
	ddOrchestratorURL = "DD_ORCHESTRATOR_EXPLORER_ORCHESTRATOR_DD_URL"
	expectedValue = "456.datadoghq.com"
	suite.T().Setenv(ddOrchestratorURL, expectedValue)
	err = orchestratorCfg.Load()
	suite.NoError(err)

	suite.Equal(expectedValue, orchestratorCfg.OrchestratorEndpoints[0].Endpoint.Path)
}

func (suite *YamlConfigTestSuite) TestEnvConfigAdditionalEndpoints() {
	suite.T().Setenv("DD_ORCHESTRATOR_ADDITIONAL_ENDPOINTS", `{"https://process1.com": ["key1"], "https://process2.com": ["key2"]}`)

	expected := map[string]string{
		"key1": "process1.com",
		"key2": "process2.com",
	}

	actualEndpoints := []apicfg.Endpoint{}
	err := extractOrchestratorAdditionalEndpoints(&url.URL{}, &actualEndpoints)
	suite.NoError(err)

	suite.Len(actualEndpoints, len(expected))
	for _, actual := range actualEndpoints {
		suite.Equal(expected[actual.APIKey], actual.Endpoint.Hostname())
	}

	// Override to make sure the precedence
	suite.T().Setenv("DD_ORCHESTRATOR_EXPLORER_ORCHESTRATOR_ADDITIONAL_ENDPOINTS", `{"https://orchestrator1.com": ["key1"], "https://orchestrator2.com": ["key2", "key3"]}`)

	expected = map[string]string{
		"key1": "orchestrator1.com",
		"key2": "orchestrator2.com",
		"key3": "orchestrator2.com",
	}

	actualEndpoints = []apicfg.Endpoint{}
	err = extractOrchestratorAdditionalEndpoints(&url.URL{}, &actualEndpoints)
	suite.NoError(err)
	suite.Len(actualEndpoints, len(expected))
	for _, actual := range actualEndpoints {
		suite.Equal(expected[actual.APIKey], actual.Endpoint.Hostname())
	}
}

func (suite *YamlConfigTestSuite) TestEnvConfigMessageSize() {
	ddMaxMessage := "DD_ORCHESTRATOR_EXPLORER_MAX_PER_MESSAGE"
	expectedValue := "50"
	suite.T().Setenv(ddMaxMessage, expectedValue)

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

	suite.T().Setenv(ddMaxMessage, "150")

	orchestratorCfg := NewDefaultOrchestratorConfig()
	err := orchestratorCfg.Load()
	suite.NoError(err)

	suite.Equal(expectedDefaultValue, orchestratorCfg.MaxPerMessage)
}

func (suite *YamlConfigTestSuite) TestEnvConfigSensitiveWords() {
	ddSensitiveWords := "DD_ORCHESTRATOR_EXPLORER_CUSTOM_SENSITIVE_WORDS"
	expectedValue := "token consul"
	suite.T().Setenv(ddSensitiveWords, expectedValue)

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
	suite.config.SetWithoutSource("orchestrator_explorer.custom_sensitive_words", `["token","consul"]`)

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
	suite.config.SetWithoutSource("orchestrator_explorer.custom_sensitive_words", `["token","consul"]`)

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
