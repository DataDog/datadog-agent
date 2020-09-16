package config

import (
	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/util/api"
	"github.com/stretchr/testify/suite"
	"net/url"
	"testing"
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
	var actualEndpoints []api.Endpoint

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
	var actualEndpoints []api.Endpoint

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
	actualEndpoints := []api.Endpoint{{APIKey: "test", Endpoint: u}}

	suite.config.Set("api_key", "wassupkey")
	suite.config.Set("process_config.orchestrator_additional_endpoints", `{"https://process1.com": ["key1"], "https://process2.com": ["key2", "key3"]}`)
	suite.config.Set("orchestrator_explorer.orchestrator_additional_endpoints", `{"https://orchestrator1.com": ["key1"], "https://orchestrator2.com": ["key2", "key3"]}`)
	err := extractOrchestratorAdditionalEndpoints(&url.URL{}, &actualEndpoints)
	suite.NoError(err)
	for _, actual := range actualEndpoints {
		suite.Equal(expected[actual.APIKey], actual.Endpoint.Hostname())
	}
}

func TestYamlConfigTestSuite(t *testing.T) {
	suite.Run(t, new(YamlConfigTestSuite))
}
