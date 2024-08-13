// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package integration

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	integrationsMock "github.com/DataDog/datadog-agent/comp/logs/integrations/mock"
	pkgConfig "github.com/DataDog/datadog-agent/pkg/config"
	// auditor "github.com/DataDog/datadog-agent/pkg/logs/auditor/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util"
	// "github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
	// "github.com/DataDog/datadog-agent/pkg/logs/tailers"
)

type LauncherTestSuite struct {
	suite.Suite
	testDir  string
	testPath string
	testFile *os.File

	outputChan       chan *message.Message
	pipelineProvider pipeline.Provider
	source           *sources.LogSource
	integrationsComp integrations.Component
	s                *Launcher
}

func (suite *LauncherTestSuite) SetupTest() {
	suite.pipelineProvider = mock.NewMockProvider()
	suite.outputChan = suite.pipelineProvider.NextPipelineChan()
	suite.integrationsComp = integrationsMock.Mock()

	var err error
	suite.testDir = suite.T().TempDir()

	suite.testPath = fmt.Sprintf("%s/launcher.log", suite.testDir)

	f, err := os.Create(suite.testPath)
	suite.Nil(err)
	suite.testFile = f

	suite.source = sources.NewLogSource("", &config.LogsConfig{Type: config.IntegrationType, Path: suite.testPath})
	suite.s = NewLauncher(nil, suite.integrationsComp)
	status.InitStatus(pkgConfig.Datadog(), util.CreateSources([]*sources.LogSource{suite.source}))
	suite.s.fileSizeMax = 1
	suite.s.runPath = suite.testDir
}

func (suite *LauncherTestSuite) TearDownTest() {
	suite.testFile.Close()
}

func (suite *LauncherTestSuite) TestFileCreation() {
	source := sources.NewLogSource("testLogsSource", &config.LogsConfig{Type: config.IntegrationType, Identifier: "123456789", Path: suite.testPath})
	sources.NewLogSources().AddSource(source)

	filePath, err := suite.s.createFile(source)
	assert.Nil(suite.T(), err)
	assert.NotNil(suite.T(), filePath)
}

// func (suite *LauncherTestSuite) TestSendLog() {
// 	logsSources := sources.NewLogSources()
// 	suite.s.sources = logsSources
// 	source := sources.NewLogSource("testLogsSource", &config.LogsConfig{Type: config.IntegrationType, Name: "integrationName", Path: suite.testPath})
// 	filepathChan := make(chan string)
// 	fileLogChan := make(chan string)
// 	suite.s.writeFunction = func(filepath, log string) error {
// 		fileLogChan <- log
// 		filepathChan <- filepath
// 		return nil
// 	}

// 	filepath, err := suite.s.createFile(source)
// 	assert.Nil(suite.T(), err)
// 	suite.s.integrationNameToFile[source.Name] = filepath
// 	fileSource := suite.s.makeFileSource(source, filepath)
// 	suite.s.sources.AddSource(fileSource)

// 	suite.s.Start(launchers.NewMockSourceProvider(), suite.pipelineProvider, auditor.NewRegistry(), tailers.NewTailerTracker())

// 	logSample := "hello world"
// 	suite.integrationsComp.SendLog(logSample, "testLogsSource:HASH1234")

// 	assert.Equal(suite.T(), logSample, <-fileLogChan)
// 	assert.Equal(suite.T(), filepath, <-filepathChan)
// }

func (suite *LauncherTestSuite) TestWriteLogToFile() {
	logText := "hello world"
	suite.s.writeLogToFileFunction(suite.testPath, logText)

	fileContents, err := os.ReadFile(suite.testPath)

	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), logText, string(fileContents))
}

// TestEnsureFileSize tests that ensureFileSize enforces the correct file sizes for launcher files
func (suite *LauncherTestSuite) TestEnsureFileSize() {
	filename := "testfile.log"
	file, err := os.Create(filename)
	assert.Nil(suite.T(), err)

	defer os.Remove(filename)
	defer file.Close()

	info, err := os.Stat(filename)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), int64(0), info.Size(), "Newly created file size not zero")

	// Write data the file and make sure ensureFileSize deletes the file for being too large
	data := make([]byte, 2*1024*1024)
	_, err = file.Write(data)
	assert.Nil(suite.T(), err)
	err = file.Sync()
	assert.Nil(suite.T(), err)

	info, err = file.Stat()
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), int64(2*1024*1024), info.Size())

	err = suite.s.deleteAndRemakeFile(filename)
	assert.Nil(suite.T(), err)

	info, err = os.Stat(filename)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), int64(0), info.Size())

// TestIntegrationLogFilePath ensures the filepath for the logs files are correct
func (suite *LauncherTestSuite) TestIntegrationLogFilePath() {
	logSource := sources.NewLogSource("testLogsSource", &config.LogsConfig{
		Type:    config.IntegrationType,
		Name:    "test_name",
		Service: "test_service",
		Path:    suite.testPath,
	})

	actualDirectory, actualFilePath := suite.s.integrationLogFilePath(*logSource)

	expectedDirectoryComponents := []string{suite.s.runPath, "integrations", logSource.Config.Service}
	expectedDirectory := strings.Join(expectedDirectoryComponents, "/")

	expectedFilePath := strings.Join([]string{expectedDirectory, logSource.Config.Name + ".log"}, "/")

	assert.Equal(suite.T(), expectedDirectory, actualDirectory)
	assert.Equal(suite.T(), expectedFilePath, actualFilePath)
}

func TestLauncherTestSuite(t *testing.T) {
	suite.Run(t, new(LauncherTestSuite))
}
