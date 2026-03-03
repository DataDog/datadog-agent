// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator && test

package config

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	model "github.com/DataDog/datadog-agent/pkg/config/model"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"github.com/DataDog/datadog-agent/pkg/redact"
)

type YamlConfigTestSuite struct {
	suite.Suite
	config model.BuildableConfig
}

func (suite *YamlConfigTestSuite) SetupTest() {
	suite.config = configmock.New(suite.T())
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

	orchestratorCfg := NewDefaultOrchestratorConfig([]string{"env:prod"})
	err := orchestratorCfg.Load()
	suite.NoError(err)

	suite.Equal(expectedValue, orchestratorCfg.OrchestratorEndpoints[0].Endpoint.Path)

	// Override to make sure the precedence
	ddOrchestratorURL = "DD_ORCHESTRATOR_EXPLORER_ORCHESTRATOR_DD_URL"
	expectedValue = "456.datadoghq.com"
	suite.T().Setenv(ddOrchestratorURL, expectedValue)
	err = orchestratorCfg.Load()
	suite.NoError(err)

	suite.Equal([]string{"env:prod"}, orchestratorCfg.ExtraTags)
	suite.Equal(expectedValue, orchestratorCfg.OrchestratorEndpoints[0].Endpoint.Path)
}

func (suite *YamlConfigTestSuite) TestEnvConfigAdditionalEndpoints() {
	suite.T().Setenv("DD_ORCHESTRATOR_ADDITIONAL_ENDPOINTS", `{"https://process1.com": ["key1"], "https://process2.com": ["key2"]}`)
	suite.config.BuildSchema()

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

	orchestratorCfg := NewDefaultOrchestratorConfig(nil)
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

	orchestratorCfg := NewDefaultOrchestratorConfig(nil)
	err := orchestratorCfg.Load()
	suite.NoError(err)

	suite.Equal(expectedDefaultValue, orchestratorCfg.MaxPerMessage)
}

func (suite *YamlConfigTestSuite) TestEnvConfigSensitiveWords() {
	ddSensitiveWords := "DD_ORCHESTRATOR_EXPLORER_CUSTOM_SENSITIVE_WORDS"
	expectedValue := "token consul"
	suite.T().Setenv(ddSensitiveWords, expectedValue)

	orchestratorCfg := NewDefaultOrchestratorConfig(nil)
	err := orchestratorCfg.Load()
	suite.NoError(err)

	for val := range strings.SplitSeq(expectedValue, " ") {
		suite.Contains(orchestratorCfg.Scrubber.LiteralSensitivePatterns, val)
	}
}

func (suite *YamlConfigTestSuite) TestEnvConfigSensitiveAnnotationsAndLabels() {
	ddSensitiveAnnotationsLabels := "DD_ORCHESTRATOR_EXPLORER_CUSTOM_SENSITIVE_ANNOTATIONS_LABELS"
	expectedValue := "my-sensitive-annotation my-sensitive-label"
	suite.T().Setenv(ddSensitiveAnnotationsLabels, expectedValue)

	orchestratorCfg := NewDefaultOrchestratorConfig(nil)
	err := orchestratorCfg.Load()
	suite.NoError(err)

	for val := range strings.SplitSeq(expectedValue, " ") {
		suite.Contains(redact.GetSensitiveAnnotationsAndLabels(), val)
	}
}

func (suite *YamlConfigTestSuite) TestNoEnvConfigArgsScrubbing() {
	orchestratorCfg := NewDefaultOrchestratorConfig(nil)
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
		actual, _, _ := orchestratorCfg.Scrubber.ScrubSimpleCommand(cases[i].cmdline, nil)
		suite.Equal(cases[i].parsedCmdline, actual)
	}
}

func (suite *YamlConfigTestSuite) TestOnlyEnvConfigArgsScrubbing() {
	suite.config.SetWithoutSource("orchestrator_explorer.custom_sensitive_words", `["token","consul"]`)

	orchestratorCfg := NewDefaultOrchestratorConfig(nil)
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
		actual, _, _ := orchestratorCfg.Scrubber.ScrubSimpleCommand(cases[i].cmdline, nil)
		suite.Equal(cases[i].parsedCmdline, actual)
	}
}

func (suite *YamlConfigTestSuite) TestOnlyEnvContainsConfigArgsScrubbing() {
	suite.config.SetWithoutSource("orchestrator_explorer.custom_sensitive_words", `["token","consul"]`)

	orchestratorCfg := NewDefaultOrchestratorConfig(nil)
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

func (suite *YamlConfigTestSuite) TestLoadFunction() {
	// Test basic Load functionality with default values
	orchestratorCfg := NewDefaultOrchestratorConfig([]string{"env:test"})
	err := orchestratorCfg.Load()
	suite.NoError(err)

	// Check that default URL is set
	expectedURL, _ := url.Parse("https://orchestrator.datadoghq.com")
	suite.Equal(expectedURL, orchestratorCfg.OrchestratorEndpoints[0].Endpoint)

	// Check that extra tags are preserved
	suite.Equal([]string{"env:test"}, orchestratorCfg.ExtraTags)

	// Check default values
	suite.Equal(100, orchestratorCfg.MaxPerMessage)
	suite.Equal(10000000, orchestratorCfg.MaxWeightPerMessageBytes)
}

func (suite *YamlConfigTestSuite) TestLoadWithAPIKey() {
	suite.config.SetWithoutSource("api_key", "test-api-key-123")

	orchestratorCfg := NewDefaultOrchestratorConfig(nil)
	err := orchestratorCfg.Load()
	suite.NoError(err)

	// Check that API key is set (it will be sanitized/hashed)
	suite.NotEmpty(orchestratorCfg.OrchestratorEndpoints[0].APIKey)
	suite.Equal("api_key", orchestratorCfg.OrchestratorEndpoints[0].ConfigSettingPath)
}

func (suite *YamlConfigTestSuite) TestLoadWithCustomURL() {
	suite.config.SetWithoutSource("orchestrator_explorer.orchestrator_dd_url", "https://custom-orchestrator.com")

	orchestratorCfg := NewDefaultOrchestratorConfig(nil)
	err := orchestratorCfg.Load()
	suite.NoError(err)

	expectedURL, _ := url.Parse("https://custom-orchestrator.com")
	suite.Equal(expectedURL, orchestratorCfg.OrchestratorEndpoints[0].Endpoint)
}

func (suite *YamlConfigTestSuite) TestLoadWithCustomSensitiveWords() {
	suite.config.SetWithoutSource("orchestrator_explorer.custom_sensitive_words", []string{"secret", "password"})

	orchestratorCfg := NewDefaultOrchestratorConfig(nil)
	err := orchestratorCfg.Load()
	suite.NoError(err)

	// Check that custom sensitive words are added to the scrubber
	suite.Contains(orchestratorCfg.Scrubber.LiteralSensitivePatterns, "secret")
	suite.Contains(orchestratorCfg.Scrubber.LiteralSensitivePatterns, "password")
}

func (suite *YamlConfigTestSuite) TestLoadWithCustomSensitiveAnnotationsLabels() {
	suite.config.SetWithoutSource("orchestrator_explorer.custom_sensitive_annotations_labels", []string{"sensitive-annotation", "secret-label"})

	orchestratorCfg := NewDefaultOrchestratorConfig(nil)
	err := orchestratorCfg.Load()
	suite.NoError(err)

	// Check that sensitive annotations and labels are updated
	sensitiveItems := redact.GetSensitiveAnnotationsAndLabels()
	suite.Contains(sensitiveItems, "sensitive-annotation")
	suite.Contains(sensitiveItems, "secret-label")
}

func (suite *YamlConfigTestSuite) TestLoadWithCustomMaxPerMessage() {
	suite.config.SetWithoutSource("orchestrator_explorer.max_per_message", 50)

	orchestratorCfg := NewDefaultOrchestratorConfig(nil)
	err := orchestratorCfg.Load()
	suite.NoError(err)

	suite.Equal(50, orchestratorCfg.MaxPerMessage)
}

func (suite *YamlConfigTestSuite) TestLoadWithInvalidMaxPerMessage() {
	// Test with value that's too high
	suite.config.SetWithoutSource("orchestrator_explorer.max_per_message", 150)

	orchestratorCfg := NewDefaultOrchestratorConfig(nil)
	err := orchestratorCfg.Load()
	suite.NoError(err)

	// Should remain at default value due to bounds checking
	suite.Equal(100, orchestratorCfg.MaxPerMessage)
}

func (suite *YamlConfigTestSuite) TestLoadWithCustomMaxMessageBytes() {
	suite.config.SetWithoutSource("orchestrator_explorer.max_message_bytes", 25000000) // 25 MB

	orchestratorCfg := NewDefaultOrchestratorConfig(nil)
	err := orchestratorCfg.Load()
	suite.NoError(err)

	suite.Equal(25000000, orchestratorCfg.MaxWeightPerMessageBytes)
}

func (suite *YamlConfigTestSuite) TestLoadWithOrchestratorEnabled() {
	suite.config.SetWithoutSource("orchestrator_explorer.enabled", true)

	orchestratorCfg := NewDefaultOrchestratorConfig(nil)
	err := orchestratorCfg.Load()
	suite.NoError(err)

	suite.True(orchestratorCfg.OrchestrationCollectionEnabled)
	// Note: KubeClusterName may be empty in test environment due to hostname resolution
}

func (suite *YamlConfigTestSuite) TestLoadWithCollectorDiscoveryEnabled() {
	suite.config.SetWithoutSource("orchestrator_explorer.collector_discovery.enabled", true)

	orchestratorCfg := NewDefaultOrchestratorConfig(nil)
	err := orchestratorCfg.Load()
	suite.NoError(err)

	suite.True(orchestratorCfg.CollectorDiscoveryEnabled)
}

func (suite *YamlConfigTestSuite) TestLoadWithScrubbingEnabled() {
	suite.config.SetWithoutSource("orchestrator_explorer.container_scrubbing.enabled", true)

	orchestratorCfg := NewDefaultOrchestratorConfig(nil)
	err := orchestratorCfg.Load()
	suite.NoError(err)

	suite.True(orchestratorCfg.IsScrubbingEnabled)
}

func (suite *YamlConfigTestSuite) TestLoadWithManifestCollection() {
	suite.config.SetWithoutSource("orchestrator_explorer.manifest_collection.enabled", true)
	suite.config.SetWithoutSource("orchestrator_explorer.manifest_collection.buffer_manifest", true)
	suite.config.SetWithoutSource("orchestrator_explorer.manifest_collection.buffer_flush_interval", "30s")

	orchestratorCfg := NewDefaultOrchestratorConfig(nil)
	err := orchestratorCfg.Load()
	suite.NoError(err)

	suite.True(orchestratorCfg.IsManifestCollectionEnabled)
	suite.True(orchestratorCfg.BufferedManifestEnabled)
	suite.Equal(30*time.Second, orchestratorCfg.ManifestBufferFlushInterval)
}

func (suite *YamlConfigTestSuite) TestLoadWithAdditionalEndpoints() {
	suite.config.SetWithoutSource("api_key", "main-api-key")
	suite.config.SetWithoutSource("orchestrator_explorer.orchestrator_additional_endpoints",
		`{"https://endpoint1.com": ["key1"], "https://endpoint2.com": ["key2", "key3"]}`)

	orchestratorCfg := NewDefaultOrchestratorConfig(nil)
	err := orchestratorCfg.Load()
	suite.NoError(err)

	// Should have main endpoint + 3 additional endpoints (key1, key2, key3)
	suite.Len(orchestratorCfg.OrchestratorEndpoints, 4)

	// Check that main endpoint has the API key (will be sanitized)
	suite.NotEmpty(orchestratorCfg.OrchestratorEndpoints[0].APIKey)

	// Check that additional endpoints are properly configured
	endpointMap := make(map[string]string)
	for _, endpoint := range orchestratorCfg.OrchestratorEndpoints[1:] { // Skip main endpoint
		endpointMap[endpoint.APIKey] = endpoint.Endpoint.Hostname()
	}

	suite.Equal("endpoint1.com", endpointMap["key1"])
	suite.Equal("endpoint2.com", endpointMap["key2"])
	suite.Equal("endpoint2.com", endpointMap["key3"])
}

func (suite *YamlConfigTestSuite) TestLoadComprehensive() {
	// Test with multiple configuration options set
	suite.config.SetWithoutSource("api_key", "comprehensive-test-key")
	suite.config.SetWithoutSource("orchestrator_explorer.orchestrator_dd_url", "https://comprehensive-test.com")
	suite.config.SetWithoutSource("orchestrator_explorer.enabled", true)
	suite.config.SetWithoutSource("orchestrator_explorer.collector_discovery.enabled", true)
	suite.config.SetWithoutSource("orchestrator_explorer.container_scrubbing.enabled", true)
	suite.config.SetWithoutSource("orchestrator_explorer.manifest_collection.enabled", true)
	suite.config.SetWithoutSource("orchestrator_explorer.max_per_message", 75)
	suite.config.SetWithoutSource("orchestrator_explorer.max_message_bytes", 30000000)
	suite.config.SetWithoutSource("orchestrator_explorer.custom_sensitive_words", []string{"token", "secret"})

	orchestratorCfg := NewDefaultOrchestratorConfig([]string{"env:comprehensive"})
	err := orchestratorCfg.Load()
	suite.NoError(err)

	// Verify all configurations are properly loaded
	expectedURL, _ := url.Parse("https://comprehensive-test.com")
	suite.Equal(expectedURL, orchestratorCfg.OrchestratorEndpoints[0].Endpoint)
	suite.NotEmpty(orchestratorCfg.OrchestratorEndpoints[0].APIKey) // API key will be sanitized
	suite.Equal([]string{"env:comprehensive"}, orchestratorCfg.ExtraTags)
	suite.True(orchestratorCfg.OrchestrationCollectionEnabled)
	suite.True(orchestratorCfg.CollectorDiscoveryEnabled)
	suite.True(orchestratorCfg.IsScrubbingEnabled)
	suite.True(orchestratorCfg.IsManifestCollectionEnabled)
	suite.Equal(75, orchestratorCfg.MaxPerMessage)
	suite.Equal(30000000, orchestratorCfg.MaxWeightPerMessageBytes)
	suite.Contains(orchestratorCfg.Scrubber.LiteralSensitivePatterns, "token")
	suite.Contains(orchestratorCfg.Scrubber.LiteralSensitivePatterns, "secret")
}
