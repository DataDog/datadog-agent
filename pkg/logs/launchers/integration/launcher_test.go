// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	integrationsmock "github.com/DataDog/datadog-agent/comp/logs/integrations/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
)

type LauncherTestSuite struct {
	suite.Suite
	testDir  string
	testPath string

	outputChan       chan *message.Message
	pipelineProvider pipeline.Provider
	source           *sources.LogSource
	integrationsComp integrations.Component
	s                *Launcher
}

func (suite *LauncherTestSuite) SetupTest() {
	suite.pipelineProvider = mock.NewMockProvider()
	suite.outputChan = suite.pipelineProvider.NextPipelineChan()
	suite.integrationsComp = integrationsmock.Mock()
	suite.testDir = suite.T().TempDir()
	suite.testPath = filepath.Join(suite.testDir, "logs_integration_test.log")

	suite.source = sources.NewLogSource(suite.T().Name(), &config.LogsConfig{Type: config.IntegrationType, Path: suite.testPath})

	// Override `logs_config.run_path` before calling `sources.NewLogSources()` as otherwise
	// it will try and create `/opt/datadog` directory and fail
	pkgconfigsetup.Datadog().SetWithoutSource("logs_config.run_path", suite.testDir)

	suite.s = NewLauncher(sources.NewLogSources(), suite.integrationsComp)
	status.InitStatus(pkgconfigsetup.Datadog(), util.CreateSources([]*sources.LogSource{suite.source}))
	suite.s.runPath = suite.testDir
}

func (suite *LauncherTestSuite) TestFileCreation() {
	id := "123456789"
	source := sources.NewLogSource("testLogsSource", &config.LogsConfig{Type: config.IntegrationType, Identifier: "123456789", Path: suite.testPath})
	sources.NewLogSources().AddSource(source)

	logFilePath, err := suite.s.createFile(id)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), logFilePath)
}

func (suite *LauncherTestSuite) TestSendLog() {

	mockConf := &integration.Config{}
	mockConf.Provider = "container"
	mockConf.LogsConfig = integration.Data(`[{"type": "integration", "source": "foo", "service": "bar"}]`)

	filepathChan := make(chan string)
	fileLogChan := make(chan string)
	suite.s.writeFunction = func(logFilePath, log string) error {
		fileLogChan <- log
		filepathChan <- logFilePath
		return nil
	}

	id := "123456789"

	suite.s.Start(nil, nil, nil, nil)
	suite.integrationsComp.RegisterIntegration(id, *mockConf)

	logSample := "hello world"
	suite.integrationsComp.SendLog(logSample, id)

	foundSource := suite.s.sources.GetSources()[0]
	assert.Equal(suite.T(), foundSource.Config.Type, config.FileType)
	assert.Equal(suite.T(), foundSource.Config.Source, "foo")
	assert.Equal(suite.T(), foundSource.Config.Service, "bar")
	expectedPath := suite.s.integrationToFile[id]

	assert.Equal(suite.T(), logSample, <-fileLogChan)
	assert.Equal(suite.T(), expectedPath, <-filepathChan)
}

func (suite *LauncherTestSuite) TestWriteLogToFile() {
	logText := "hello world"
	err := suite.s.writeFunction(suite.testPath, logText)
	require.Nil(suite.T(), err)

	fileContents, err := os.ReadFile(suite.testPath)

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), logText+"\n", string(fileContents))
}

func (suite *LauncherTestSuite) TestWriteMultipleLogsToFile() {
	var err error
	err = suite.s.writeFunction(suite.testPath, "line 1")
	require.Nil(suite.T(), err, "error writing line 1")

	err = suite.s.writeFunction(suite.testPath, "line 2")
	require.Nil(suite.T(), err, "error writing line 2")

	err = suite.s.writeFunction(suite.testPath, "line 3")
	require.Nil(suite.T(), err, "error writing line 3")

	fileContents, err := os.ReadFile(suite.testPath)

	assert.NoError(suite.T(), err)
	expectedContent := "line 1\nline 2\nline 3\n"
	assert.Equal(suite.T(), expectedContent, string(fileContents))
}

// TestIntegrationLogFilePath ensures the filepath for the logs files are correct
func (suite *LauncherTestSuite) TestIntegrationLogFilePath() {
	id := "123456789"
	actualFilePath := suite.s.integrationLogFilePath(id)
	expectedFilePath := filepath.Join(suite.s.runPath, id+".log")
	assert.Equal(suite.T(), expectedFilePath, actualFilePath)

	id = "1234 5678:myIntegration"
	actualFilePath = suite.s.integrationLogFilePath(id)
	expectedFilePath = filepath.Join(suite.s.runPath, "1234-5678_myIntegration.log")
	assert.Equal(suite.T(), expectedFilePath, actualFilePath)
}

func TestLauncherTestSuite(t *testing.T) {
	suite.Run(t, new(LauncherTestSuite))
}
