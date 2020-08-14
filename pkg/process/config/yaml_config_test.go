package config

import (
	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
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
	expected, err := url.Parse("https://orchestrator-link.com")
	suite.NoError(err)
	suite.Equal(expected, actual)
}

func (suite *YamlConfigTestSuite) TestExtractOrchestratorDDProcessUrl() {
	suite.config.Set("api_key", "wassupkey")
	suite.config.Set("process_config.orchestrator_dd_url", "https://process-link.com")
	actual, err := extractOrchestratorDDUrl()
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
	expected, err := url.Parse("https://orchestrator-link.com")
	suite.NoError(err)
	suite.Equal(expected, actual)
}

func (suite *YamlConfigTestSuite) TestExtractOrchestratorProcessEndpoints() {
	suite.config.Set("api_key", "wassupkey")
	suite.config.Set("process_config.orchestrator_dd_url", "https://process-link.com")
	suite.config.Set("orchestrator_explorer.orchestrator_dd_url", "https://orchestrator-link.com")
	actual, err := extractOrchestratorDDUrl()
	expected, err := url.Parse("https://orchestrator-link.com")
	suite.NoError(err)
	suite.Equal(expected, actual)
}

func (suite *YamlConfigTestSuite) TestExtractOrchestratorEndpoints() {
	suite.config.Set("api_key", "wassupkey")
	suite.config.Set("process_config.orchestrator_dd_url", "https://process-link.com")
	suite.config.Set("orchestrator_explorer.orchestrator_dd_url", "https://orchestrator-link.com")
	actual, err := extractOrchestratorDDUrl()
	expected, err := url.Parse("https://orchestrator-link.com")
	suite.NoError(err)
	suite.Equal(expected, actual)
}

func (suite *YamlConfigTestSuite) TestExtractOrchestratorEndpointsPrecedence() {
	suite.config.Set("api_key", "wassupkey")
	suite.config.Set("process_config.orchestrator_dd_url", "https://process-link.com")
	suite.config.Set("orchestrator_explorer.orchestrator_dd_url", "https://orchestrator-link.com")
	actual, err := extractOrchestratorDDUrl()
	expected, err := url.Parse("https://orchestrator-link.com")
	suite.NoError(err)
	suite.Equal(expected, actual)
}

func TestYamlConfigTestSuite(t *testing.T) {
	suite.Run(t, new(YamlConfigTestSuite))
}
