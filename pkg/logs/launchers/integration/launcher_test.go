// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package integration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/afero"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	integrationsmock "github.com/DataDog/datadog-agent/comp/logs/integrations/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/logs/util/testutils"
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
	fs               afero.Fs
}

func (suite *LauncherTestSuite) SetupTest() {
	cfg := configmock.New(suite.T())

	suite.fs = afero.NewMemMapFs()
	suite.pipelineProvider = mock.NewMockProvider()
	suite.outputChan = suite.pipelineProvider.NextPipelineChan()
	suite.integrationsComp = integrationsmock.Mock()
	suite.testDir = suite.T().TempDir()
	suite.testPath = filepath.Join(suite.testDir, "logs_integration_test.log")

	suite.source = sources.NewLogSource(suite.T().Name(), &config.LogsConfig{Type: config.IntegrationType, Path: suite.testPath})
	// Override `logs_config.run_path` before calling `sources.NewLogSources()` as otherwise
	// it will try and create `/opt/datadog` directory and fail
	cfg.SetWithoutSource("logs_config.run_path", suite.testDir)

	suite.s = NewLauncher(suite.fs, sources.NewLogSources(), suite.integrationsComp)
	suite.s.fileSizeMax = 10 * 1024 * 1024
	status.InitStatus(cfg, testutils.CreateSources([]*sources.LogSource{suite.source}))
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
	suite.s.writeLogToFileFunction = func(_ afero.Fs, logFilePath, log string) error {
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
	expectedPath := suite.s.integrationToFile[id].fileWithPath

	assert.Equal(suite.T(), logSample, <-fileLogChan)
	assert.Equal(suite.T(), expectedPath, <-filepathChan)
}

func (suite *LauncherTestSuite) TestEmptyConfig() {
	mockConf := &integration.Config{}
	mockConf.Provider = "container"
	mockConf.LogsConfig = integration.Data(``)

	suite.s.Start(nil, nil, nil, nil)
	suite.integrationsComp.RegisterIntegration("12345", *mockConf)

	assert.Equal(suite.T(), len(suite.s.sources.GetSources()), 0)
}

// TestNegativeCombinedUsageMax ensures errors in combinedUsageMax don't result
// in panics from `deleteFile`
func (suite *LauncherTestSuite) TestNegativeCombinedUsageMax() {
	suite.s.combinedUsageMax = -1
	err := suite.s.scanInitialFiles(suite.s.runPath)
	assert.Error(suite.T(), err)
}

// TestZeroCombinedUsageMax ensures the launcher won't panic when
// combinedUsageMax is zero. Realistically the launcher would never run receiveLogs since there is a check for
func (suite *LauncherTestSuite) TestZeroCombinedUsageMaxFileCreated() {
	suite.s.combinedUsageMax = 0

	filename := "sample_integration_123.log"
	fileWithPath := filepath.Join(suite.s.runPath, filename)
	file, err := suite.fs.Create(fileWithPath)
	assert.NoError(suite.T(), err)

	file.Close()

	suite.s.Start(nil, nil, nil, nil)

	integrationLog := integrations.IntegrationLog{
		Log:           "sample log",
		IntegrationID: "sample_integration:123",
	}

	suite.s.receiveLogs(integrationLog)
}

func (suite *LauncherTestSuite) TestZeroCombinedUsageMaxFileNotCreated() {
	suite.s.combinedUsageMax = 0

	suite.s.Start(nil, nil, nil, nil)

	integrationLog := integrations.IntegrationLog{
		Log:           "sample log",
		IntegrationID: "sample_integration:123",
	}

	suite.s.receiveLogs(integrationLog)
}

func (suite *LauncherTestSuite) TestSmallCombinedUsageMax() {
	suite.s.combinedUsageMax = 15

	filename := "sample_integration_123.log"
	fileWithPath := filepath.Join(suite.s.runPath, filename)
	file, err := suite.fs.Create(fileWithPath)
	assert.NoError(suite.T(), err)

	file.Close()

	suite.s.Start(nil, nil, nil, nil)

	// Launcher should write this log
	shortLog := "sample"
	integrationLog := integrations.IntegrationLog{
		Log:           shortLog,
		IntegrationID: "sample_integration:123",
	}
	suite.s.receiveLogs(integrationLog)
	fileStat, err := suite.fs.Stat(fileWithPath)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), fileStat.Size(), int64(len(shortLog)+1))

	// Launcher should delete and remake the log file for this log since it would break combinedUsageMax threshold
	longLog := "sample log two"
	integrationLogTwo := integrations.IntegrationLog{
		Log:           longLog,
		IntegrationID: "sample_integration:123",
	}
	suite.s.receiveLogs(integrationLogTwo)
	_, err = suite.fs.Stat(fileWithPath)
	assert.NoError(suite.T(), err)

	// Launcher should skip writing this log since it's larger than combinedUsageMax
	unwrittenLog := "this log is too long"
	unwrittenIntegrationLog := integrations.IntegrationLog{
		Log:           unwrittenLog,
		IntegrationID: "sample_integration:123",
	}
	suite.s.receiveLogs(unwrittenIntegrationLog)
	_, err = suite.fs.Stat(fileWithPath)
	assert.NoError(suite.T(), err)

	// Remake the file
	suite.s.receiveLogs(integrationLog)
	fileStat, err = suite.fs.Stat(fileWithPath)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), fileStat.Size(), int64(len(shortLog)+1))
}

func (suite *LauncherTestSuite) TestWriteLogToFile() {
	logText := "hello world"
	err := suite.s.writeLogToFileFunction(suite.fs, suite.testPath, logText)
	require.Nil(suite.T(), err)

	fileContents, err := afero.ReadFile(suite.s.fs, suite.testPath)

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), logText+"\n", string(fileContents))
}

func (suite *LauncherTestSuite) TestWriteMultipleLogsToFile() {
	var err error
	err = suite.s.writeLogToFileFunction(suite.fs, suite.testPath, "line 1")
	require.Nil(suite.T(), err, "error writing line 1")

	err = suite.s.writeLogToFileFunction(suite.fs, suite.testPath, "line 2")
	require.Nil(suite.T(), err, "error writing line 2")

	err = suite.s.writeLogToFileFunction(suite.fs, suite.testPath, "line 3")
	require.Nil(suite.T(), err, "error writing line 3")

	fileContents, err := afero.ReadFile(suite.fs, suite.testPath)

	assert.NoError(suite.T(), err)
	expectedContent := "line 1\nline 2\nline 3\n"
	assert.Equal(suite.T(), expectedContent, string(fileContents))
}

// TestDeleteFile tests that deleteFile properly deletes the correct file
func (suite *LauncherTestSuite) TestDeleteFile() {
	filename := "testfile.log"
	fileWithPath := filepath.Join(suite.s.runPath, filename)
	file, err := suite.fs.Create(fileWithPath)
	fileinfo := &fileInfo{fileWithPath: fileWithPath, size: int64(0)}
	assert.NoError(suite.T(), err)

	info, err := suite.fs.Stat(fileWithPath)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(0), info.Size(), "Newly created file size not zero")

	// Write data the file and make sure ensureFileSize deletes the file for being too large
	data := make([]byte, 2*1024*1024)
	file.Write(data)
	file.Close()

	info, err = suite.fs.Stat(fileWithPath)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(2*1024*1024), info.Size())

	err = suite.s.deleteFile(fileinfo)
	assert.NoError(suite.T(), err)

	_, err = suite.fs.Stat(fileWithPath)
	assert.True(suite.T(), os.IsNotExist(err))
}

// TestIntegrationLogFilePath ensures the filepath for the logs files are correct
func (suite *LauncherTestSuite) TestIntegrationLogFilePath() {
	id := "123456789"
	actualFilePath := suite.s.integrationLogFilePath(id)
	expectedFilePath := filepath.Join(suite.s.runPath, id+".log")
	assert.Equal(suite.T(), expectedFilePath, actualFilePath)

	id = "1234 5678:myIntegration"
	actualFilePath = suite.s.integrationLogFilePath(id)
	expectedFilePath = filepath.Join(suite.s.runPath, "1234 5678_myIntegration.log")
	assert.Equal(suite.T(), expectedFilePath, actualFilePath)
}

// TestFileNameToID ensures file names are decoded to their proper id
func (suite *LauncherTestSuite) TestFileNameToID() {
	tests := []struct {
		input    string
		expected string
	}{
		{"file_name_1234.log", "file_name:1234"},
		{"example_test_5678abcd.log", "example_test:5678abcd"},
		{"integration with spaces_5678.log", "integration with spaces:5678"},
		{"file_with_multiple_underscores_9999.log", "file_with_multiple_underscores:9999"},
	}

	for _, tt := range tests {
		suite.T().Run(tt.input, func(_ *testing.T) {
			result := fileNameToID(tt.input)
			assert.Equal(suite.T(), tt.expected, result)
		})
	}
}

// TestFileExceedsSingleFileLimit ensures individual files cannot exceed file
// limit sizes
func (suite *LauncherTestSuite) TestFileExceedsSingleFileLimit() {
	oneMB := int64(1 * 1024 * 1024)
	suite.s.combinedUsageMax = 2 * oneMB
	suite.s.fileSizeMax = oneMB

	filename := "sample_integration_123.log"
	fileWithPath := filepath.Join(suite.s.runPath, filename)
	file, err := suite.fs.Create(fileWithPath)
	assert.NoError(suite.T(), err)

	file.Write(make([]byte, oneMB))
	file.Close()

	suite.s.Start(nil, nil, nil, nil)

	integrationLog := integrations.IntegrationLog{
		Log:           "sample log",
		IntegrationID: "sample_integration:123",
	}

	suite.s.receiveLogs(integrationLog)

	assert.Equal(suite.T(), int64(len(integrationLog.Log)+1), suite.s.combinedUsageSize)
	assert.Equal(suite.T(), int64(len(integrationLog.Log)+1), suite.s.integrationToFile["sample_integration:123"].size)
	assert.Equal(suite.T(), 1, len(suite.s.integrationToFile))
}

// TestScanInitialFiles ensures files already present in the runPath for the
// launcher are detected and managed upon launcher start
func (suite *LauncherTestSuite) TestScanInitialFiles() {
	filename := "sample_integration_123.log"
	fileSize := int64(1 * 1024 * 1024)

	fileWithPath := filepath.Join(suite.s.runPath, filename)
	file, err := suite.fs.Create(fileWithPath)
	assert.NoError(suite.T(), err)

	data := make([]byte, fileSize)
	file.Write(data)
	file.Close()

	suite.s.scanInitialFiles(suite.s.runPath)
	fileID := fileNameToID(filename)
	actualFileInfo := suite.s.integrationToFile[fileID]

	assert.NotEmpty(suite.T(), suite.s.integrationToFile)
	assert.Equal(suite.T(), actualFileInfo.fileWithPath, fileWithPath)
	assert.Equal(suite.T(), fileSize, actualFileInfo.size)
	assert.Equal(suite.T(), fileSize, suite.s.combinedUsageSize)
}

// TestCreateFileAfterScanInitialFile ensures files tracked by scanInitialFiles
// are not created again after they've already been scanned
func (suite *LauncherTestSuite) TestCreateFileAfterScanInitialFile() {
	filename := "sample_integration_123.log"
	fileSize := int64(1 * 1024 * 1024)

	fileWithPath := filepath.Join(suite.s.runPath, filename)
	file, err := suite.fs.Create(fileWithPath)
	assert.NoError(suite.T(), err)

	data := make([]byte, fileSize)
	file.Write(data)
	file.Close()

	suite.s.scanInitialFiles(suite.s.runPath)
	fileID := fileNameToID(filename)
	scannedFile := suite.s.integrationToFile[fileID]

	assert.NotEmpty(suite.T(), suite.s.integrationToFile)
	assert.Equal(suite.T(), fileWithPath, scannedFile.fileWithPath)
	assert.Equal(suite.T(), fileSize, scannedFile.size)
	assert.Equal(suite.T(), fileSize, suite.s.combinedUsageSize)

	mockConf := &integration.Config{}
	mockConf.Provider = "container"
	mockConf.LogsConfig = integration.Data(`[{"type": "integration", "source": "foo", "service": "bar"}]`)

	filepathChan := make(chan string)
	fileLogChan := make(chan string)
	suite.s.writeLogToFileFunction = func(_ afero.Fs, logFilePath, log string) error {
		fileLogChan <- log
		filepathChan <- logFilePath
		return nil
	}

	suite.s.Start(nil, nil, nil, nil)
	suite.integrationsComp.RegisterIntegration(fileID, *mockConf)
	assert.Equal(suite.T(), 1, len(suite.s.integrationToFile))

	logSample := "hello world"
	suite.integrationsComp.SendLog(logSample, fileID)

	foundSource := suite.s.sources.GetSources()[0]
	assert.Equal(suite.T(), foundSource.Config.Type, config.FileType)
	assert.Equal(suite.T(), foundSource.Config.Source, "foo")
	assert.Equal(suite.T(), foundSource.Config.Service, "bar")

	assert.Equal(suite.T(), logSample, <-fileLogChan)
}

// TestSentLogExceedsTotalUsage ensures files are deleted when a sent log causes a
// disk usage overage
func (suite *LauncherTestSuite) TestSentLogExceedsTotalUsage() {
	suite.s.combinedUsageMax = 3 * 1024 * 1024

	// Given 3 files exist
	fileWithPath1 := filepath.Join(suite.s.runPath, "sample_integration1_123.log")
	fileWithPath2 := filepath.Join(suite.s.runPath, "sample_integration2_123.log")
	fileWithPath3 := filepath.Join(suite.s.runPath, "sample_integration3_123.log")
	fileNames := [3]string{fileWithPath1, fileWithPath2, fileWithPath3}

	//  And I write 1Mb to each file in seq order
	dataOneMB := make([]byte, 1*1024*1024)
	for _, fileWithPath := range fileNames {
		file, err := suite.fs.Create(fileWithPath)
		require.NoError(suite.T(), err)
		_, _ = file.Write(dataOneMB)
		_ = file.Close()
	}

	// If the files have the same timestamp, scanInitialFiles will detect them in
	// random order. Setting their modified time manually allows the
	// scanInitialFiles function to detect them in a deterministic manner
	modTime := time.Now()
	accessTime := time.Now()
	suite.fs.Chtimes(fileWithPath1, accessTime, modTime.Add(-2*time.Minute))
	suite.fs.Chtimes(fileWithPath2, accessTime, modTime.Add(-1*time.Minute))
	suite.fs.Chtimes(fileWithPath3, accessTime, modTime)

	suite.s.Start(nil, nil, nil, nil)

	integrationLog := integrations.IntegrationLog{
		Log:           "sample log",
		IntegrationID: "sample_integration1:123",
	}

	// When a log line is written to sample_integration1_123
	suite.s.receiveLogs(integrationLog)

	var actualSize int64
	for _, fileWithPath := range fileNames {
		file, err := suite.fs.Stat(fileWithPath)
		require.Nil(suite.T(), err)
		actualSize += file.Size()
	}

	// Then combined file size is greater than 0
	assert.Greater(suite.T(), actualSize, int64(0), "Actual combined file size should be greater than zero")
	assert.Equal(suite.T(), suite.s.combinedUsageSize, actualSize)
	// And sample_integration2 should be the least recently modified file
	// as sample_integration1_123 & sample_integration3_123 are most recently written files
	assert.Equal(suite.T(), suite.s.integrationToFile["sample_integration2:123"], suite.s.getLeastRecentlyModifiedFile())
}

// TestInitialLogsExceedTotalUsageMultipleFiles ensures initial files are deleted if they
// exceed total allowed disk space
func (suite *LauncherTestSuite) TestInitialLogsExceedTotalUsageMultipleFiles() {
	oneMB := int64(1 * 1024 * 1024)
	suite.s.combinedUsageMax = oneMB

	filename1 := "sample_integration1_123.log"
	filename2 := "sample_integration2_123.log"

	dataOneMB := make([]byte, oneMB)

	file1, err := suite.fs.Create(filepath.Join(suite.s.runPath, filename1))
	assert.NoError(suite.T(), err)
	file2, err := suite.fs.Create(filepath.Join(suite.s.runPath, filename2))
	assert.NoError(suite.T(), err)

	file1.Write(dataOneMB)
	file2.Write(dataOneMB)
	file1.Close()
	file2.Close()

	suite.s.Start(nil, nil, nil, nil)

	assert.Equal(suite.T(), oneMB, suite.s.combinedUsageSize)
	assert.Equal(suite.T(), 2, len(suite.s.integrationToFile))
}

// TestInitialLogExceedsTotalUsageSingleFile ensures an initial file won't
// exceed the total allowed disk usage space
func (suite *LauncherTestSuite) TestInitialLogExceedsTotalUsageSingleFile() {
	oneMB := int64(1 * 1024 * 1024)
	suite.s.combinedUsageMax = oneMB

	filename := "sample_integration1_123.log"
	dataTwoMB := make([]byte, 2*oneMB)

	file, err := suite.fs.Create(filepath.Join(suite.s.runPath, filename))
	assert.NoError(suite.T(), err)

	file.Write(dataTwoMB)
	file.Close()

	suite.s.Start(nil, nil, nil, nil)

	assert.Equal(suite.T(), int64(0), suite.s.combinedUsageSize)
	assert.Equal(suite.T(), 1, len(suite.s.integrationToFile))
}

// TestScanInitialFilesDeletesProperly ensures the scanInitialFiles function
// properly deletes log files once the sum of sizes for the scanned files is too
// large
func (suite *LauncherTestSuite) TestScanInitialFilesDeletesProperly() {
	err := suite.fs.RemoveAll(suite.s.runPath)
	assert.NoError(suite.T(), err)
	suite.fs.MkdirAll(suite.s.runPath, 0755)
	assert.NoError(suite.T(), err)

	oneMB := int64(1 * 1024 * 1024)
	suite.s.combinedUsageMax = oneMB

	filename1 := "sample_integration1_123.log"
	filename2 := "sample_integration2_123.log"

	name := filepath.Join(suite.s.runPath, filename1)
	file1, err := suite.fs.Create(name)
	assert.NoError(suite.T(), err)
	file2, err := suite.fs.Create(filepath.Join(suite.s.runPath, filename2))
	assert.NoError(suite.T(), err)

	dataOneMB := make([]byte, oneMB)
	file1.Write(dataOneMB)
	file2.Write(dataOneMB)
	file1.Close()
	file2.Close()

	suite.s.scanInitialFiles(suite.s.runPath)

	// make sure there is only one file in the directory
	files, err := afero.ReadDir(suite.s.fs, suite.s.runPath)
	assert.NoError(suite.T(), err)

	fileCount := 0
	for _, file := range files {
		if !file.IsDir() {
			fileCount++
		}
	}

	assert.Equal(suite.T(), 1, fileCount)
}

// TestFileRotationDeleteAndRecreate ensures that when a file exceeds the size limit,
// the launcher deletes the old file and creates a new one (delete-and-recreate rotation).
// This prevents data loss on Linux where the tailer can continue reading the deleted file
// via its open file descriptor until it reaches EOF.
func (suite *LauncherTestSuite) TestFileRotationDeleteAndRecreate() {
	// Set up a small file size limit to trigger rotation
	suite.s.fileSizeMax = 100 // 100 bytes
	suite.s.combinedUsageMax = 1000

	filename := "sample_integration_123.log"
	fileWithPath := filepath.Join(suite.s.runPath, filename)

	// Create initial file with data close to the limit
	initialData := make([]byte, 80) // 80 bytes
	for i := range initialData {
		initialData[i] = 'A'
	}
	file, err := suite.fs.Create(fileWithPath)
	assert.NoError(suite.T(), err)
	_, err = file.Write(initialData)
	assert.NoError(suite.T(), err)
	file.Close()

	// Track the file in the launcher
	suite.s.integrationToFile = make(map[string]*fileInfo)
	suite.s.integrationToFile["sample_integration:123"] = &fileInfo{
		fileWithPath: fileWithPath,
		size:         int64(len(initialData)),
		lastModified: time.Now(),
	}
	suite.s.combinedUsageSize = int64(len(initialData))

	// Send a log that will exceed the file size limit
	logThatExceedsLimit := make([]byte, 30) // 80 + 30 = 110 > 100
	for i := range logThatExceedsLimit {
		logThatExceedsLimit[i] = 'B'
	}

	integrationLog := integrations.IntegrationLog{
		Log:           string(logThatExceedsLimit),
		IntegrationID: "sample_integration:123",
	}

	// Receive the log - this should trigger rotation (delete and recreate)
	suite.s.receiveLogs(integrationLog)

	// Verify the file still exists at the same path (recreated)
	fileInfo, err := suite.fs.Stat(fileWithPath)
	assert.NoError(suite.T(), err, "File should exist after rotation")

	// Verify the file size matches only the new log (not old + new)
	expectedSize := int64(len(logThatExceedsLimit) + 1) // +1 for newline
	assert.Equal(suite.T(), expectedSize, fileInfo.Size(), "File should contain only new log after rotation")

	// Verify the file contents are only the new log (old data was deleted)
	fileContents, err := afero.ReadFile(suite.fs, fileWithPath)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), string(logThatExceedsLimit)+"\n", string(fileContents), "File should contain only new log")

	// Verify the launcher's internal tracking was updated
	assert.Equal(suite.T(), expectedSize, suite.s.integrationToFile["sample_integration:123"].size)
	assert.Equal(suite.T(), expectedSize, suite.s.combinedUsageSize)
}

// TestMultipleRotations ensures the launcher can handle multiple rotations
// for the same integration file without issues
func (suite *LauncherTestSuite) TestMultipleRotations() {
	suite.s.fileSizeMax = 100 // File size limit
	suite.s.combinedUsageMax = 1000

	filename := "sample_integration_456.log"
	fileWithPath := filepath.Join(suite.s.runPath, filename)

	// Initialize with file that has 80 bytes (close to limit)
	suite.s.integrationToFile = make(map[string]*fileInfo)
	initialData := make([]byte, 80)
	for i := range initialData {
		initialData[i] = 'I'
	}

	// Create initial file with data
	file, err := suite.fs.Create(fileWithPath)
	assert.NoError(suite.T(), err)
	file.Write(initialData)
	file.Close()

	suite.s.integrationToFile["sample_integration:456"] = &fileInfo{
		fileWithPath: fileWithPath,
		size:         int64(len(initialData)),
		lastModified: time.Now(),
	}
	suite.s.combinedUsageSize = int64(len(initialData))

	// Send 5 logs, each should trigger a rotation
	// After rotation, file has the previous log (~70 bytes + newline = 71 bytes)
	// New log is 70 bytes, so 71 + 70 = 141 > 100, triggers rotation
	for i := 0; i < 5; i++ {
		log := make([]byte, 70) // Large enough that when added to existing, always triggers rotation
		for j := range log {
			log[j] = byte('0' + i) // Different content each time
		}

		integrationLog := integrations.IntegrationLog{
			Log:           string(log),
			IntegrationID: "sample_integration:456",
		}

		suite.s.receiveLogs(integrationLog)

		// After each rotation, verify the file contains only the latest log
		fileContents, err := afero.ReadFile(suite.fs, fileWithPath)
		assert.NoError(suite.T(), err)
		assert.Equal(suite.T(), string(log)+"\n", string(fileContents),
			"After rotation %d, file should contain only the latest log", i+1)
	}

	// Verify final state
	fileInfo, err := suite.fs.Stat(fileWithPath)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(71), fileInfo.Size()) // 70 bytes + newline
}

// TestRotationPreservesOtherFiles ensures rotation of one file doesn't affect other integration files
func (suite *LauncherTestSuite) TestRotationPreservesOtherFiles() {
	suite.s.fileSizeMax = 100
	suite.s.combinedUsageMax = 1000

	// Create two integration files
	file1Path := filepath.Join(suite.s.runPath, "integration1_abc.log")
	file2Path := filepath.Join(suite.s.runPath, "integration2_def.log")

	suite.s.integrationToFile = make(map[string]*fileInfo)

	// Write initial data to both files
	initialData1 := "Initial log for integration 1"
	initialData2 := "Initial log for integration 2"

	// Create initial empty files
	file1, err := suite.fs.Create(file1Path)
	assert.NoError(suite.T(), err)
	file1.Close()

	file2, err := suite.fs.Create(file2Path)
	assert.NoError(suite.T(), err)
	file2.Close()

	suite.s.integrationToFile["integration1:abc"] = &fileInfo{
		fileWithPath: file1Path,
		size:         0,
		lastModified: time.Now(),
	}
	suite.s.integrationToFile["integration2:def"] = &fileInfo{
		fileWithPath: file2Path,
		size:         0,
		lastModified: time.Now(),
	}

	// Write to file 1
	log1 := integrations.IntegrationLog{
		Log:           initialData1,
		IntegrationID: "integration1:abc",
	}
	suite.s.receiveLogs(log1)

	// Write to file 2
	log2 := integrations.IntegrationLog{
		Log:           initialData2,
		IntegrationID: "integration2:def",
	}
	suite.s.receiveLogs(log2)

	// Trigger rotation on file 1 by sending a log that will exceed limit
	// Current file1 size: 30 bytes (initialData1) + 1 (newline) = 31 bytes
	// Send 80 byte log: 31 + 80 = 111 > 100, triggers rotation
	largeLog := make([]byte, 80)
	for i := range largeLog {
		largeLog[i] = 'X'
	}
	rotationLog := integrations.IntegrationLog{
		Log:           string(largeLog),
		IntegrationID: "integration1:abc",
	}
	suite.s.receiveLogs(rotationLog)

	// Verify file 1 was rotated (contains only new log)
	file1Contents, err := afero.ReadFile(suite.fs, file1Path)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), string(largeLog)+"\n", string(file1Contents))

	// Verify file 2 was NOT affected (still has original data)
	file2Contents, err := afero.ReadFile(suite.fs, file2Path)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), initialData2+"\n", string(file2Contents))
}

func TestLauncherTestSuite(t *testing.T) {
	suite.Run(t, new(LauncherTestSuite))
}

// TestReadOnlyFileSystem ensures the launcher doesn't panic in a read-only
// file system. There will be errors but it should handle them gracefully.
func TestReadOnlyFileSystem(t *testing.T) {
	cfg := configmock.New(t)

	fs := afero.NewMemMapFs()
	readOnlyDir, err := afero.TempDir(fs, "readonly", t.Name())
	assert.NoError(t, err, "Unable to make tempdir readonly")

	cfg.SetWithoutSource("logs_config.run_path", readOnlyDir)

	integrationsComp := integrationsmock.Mock()
	s := NewLauncher(afero.NewReadOnlyFs(fs), sources.NewLogSources(), integrationsComp)

	// Check the launcher doesn't block on receiving channels
	mockConf := &integration.Config{}
	mockConf.Provider = "container"
	mockConf.LogsConfig = integration.Data(`[{"type": "integration", "source": "foo", "service": "bar"}]`)
	id := "123456789"

	s.Start(nil, nil, nil, nil)
	integrationsComp.RegisterIntegration(id, *mockConf)

	logSample := "hello world"
	integrationsComp.SendLog(logSample, id)

	// send a second log to make sure the launcher isn't blocking
	integrationsComp.SendLog(logSample, id)
}

// TestCombinedDiskUsageFallback ensures the launcher falls back to the
// logsTotalUsageSetting if there is an error in the logsUsageRatio
func TestCombinedDiskUsageFallback(t *testing.T) {
	cfg := configmock.New(t)
	totalUsage := 100
	cfg.SetWithoutSource("logs_config.integrations_logs_disk_ratio", -1)
	cfg.SetWithoutSource("logs_config.integrations_logs_total_usage", totalUsage)

	integrationsComp := integrationsmock.Mock()
	s := NewLauncher(afero.NewOsFs(), sources.NewLogSources(), integrationsComp)

	assert.Equal(t, s.combinedUsageMax, int64(totalUsage*1024*1024))
}
