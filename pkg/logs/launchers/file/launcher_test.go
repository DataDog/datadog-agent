// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package file

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	flareController "github.com/DataDog/datadog-agent/comp/logs/agent/flare"
	auditorMock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	fileprovider "github.com/DataDog/datadog-agent/pkg/logs/launchers/file/provider"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	filetailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
)

type LauncherTestSuite struct {
	suite.Suite
	configID        string
	testDir         string
	testPath        string
	testFile        *os.File
	testRotatedPath string
	testRotatedFile *os.File

	outputChan       chan *message.Message
	pipelineProvider pipeline.Provider
	source           *sources.LogSource
	openFilesLimit   int
	s                *Launcher
	tagger           taggermock.Mock
}

func (suite *LauncherTestSuite) SetupTest() {
	cfg := configmock.New(suite.T())
	suite.pipelineProvider = mock.NewMockProvider()
	suite.outputChan = suite.pipelineProvider.NextPipelineChan()
	suite.tagger = taggerfxmock.SetupFakeTagger(suite.T())

	var err error
	suite.testDir = suite.T().TempDir()

	suite.testPath = fmt.Sprintf("%s/launcher.log", suite.testDir)
	suite.testRotatedPath = fmt.Sprintf("%s.1", suite.testPath)

	f, err := os.Create(suite.testPath)
	suite.Nil(err)
	suite.testFile = f
	f, err = os.Create(suite.testRotatedPath)
	suite.Nil(err)
	suite.testRotatedFile = f

	suite.openFilesLimit = 100
	suite.source = sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Identifier: suite.configID, Path: suite.testPath})
	sleepDuration := 20 * time.Millisecond
	fc := flareController.NewFlareController()
	suite.s = NewLauncher(suite.openFilesLimit, sleepDuration, false, 10*time.Second, "by_name", fc, suite.tagger)
	suite.s.pipelineProvider = suite.pipelineProvider
	suite.s.registry = auditorMock.NewMockRegistry()
	suite.s.activeSources = append(suite.s.activeSources, suite.source)
	status.InitStatus(cfg, util.CreateSources([]*sources.LogSource{suite.source}))
	suite.s.scan()
}

func (suite *LauncherTestSuite) TearDownTest() {
	status.Clear()
	suite.testFile.Close()
	suite.testRotatedFile.Close()
	suite.s.cleanup()
}

func (suite *LauncherTestSuite) TestLauncherStartsTailers() {
	_, err := suite.testFile.WriteString("hello world\n")
	suite.Nil(err)
	msg := <-suite.outputChan
	suite.Equal("hello world", string(msg.GetContent()))
}

func (suite *LauncherTestSuite) TestLauncherScanWithoutLogRotation() {
	s := suite.s

	var tailer *filetailer.Tailer
	var newTailer *filetailer.Tailer
	var err error
	var msg *message.Message

	tailer, _ = s.tailers.Get(getScanKey(suite.testPath, suite.source))
	_, err = suite.testFile.WriteString("hello world\n")
	suite.Nil(err)
	msg = <-suite.outputChan
	suite.Equal("hello world", string(msg.GetContent()))

	s.scan()
	newTailer, _ = s.tailers.Get(getScanKey(suite.testPath, suite.source))
	// testing that launcher did not have to create a new tailer
	suite.True(tailer == newTailer)

	_, err = suite.testFile.WriteString("hello again\n")
	suite.Nil(err)
	msg = <-suite.outputChan
	suite.Equal("hello again", string(msg.GetContent()))
}

func (suite *LauncherTestSuite) TestLauncherScanWithLogRotation() {
	s := suite.s

	var tailer *filetailer.Tailer
	var newTailer *filetailer.Tailer
	var err error
	var msg *message.Message

	_, err = suite.testFile.WriteString("hello world\n")
	suite.Nil(err)
	msg = <-suite.outputChan
	suite.Equal("hello world", string(msg.GetContent()))

	tailer, _ = s.tailers.Get(getScanKey(suite.testPath, suite.source))
	os.Rename(suite.testPath, suite.testRotatedPath)
	f, err := os.Create(suite.testPath)
	suite.Nil(err)
	s.scan()
	newTailer, _ = s.tailers.Get(getScanKey(suite.testPath, suite.source))
	suite.True(tailer != newTailer)

	_, err = f.WriteString("hello again\n")
	suite.Nil(err)
	msg = <-suite.outputChan
	suite.Equal("hello again", string(msg.GetContent()))
}

func (suite *LauncherTestSuite) TestLauncherScanWithLogRotationAndChecksum_RotationOccurs() {
	suite.s.cleanup()
	mockConfig := configmock.New(suite.T())
	mockConfig.SetWithoutSource("logs_config.fingerprint_strategy", "checksum")
	mockConfig.SetWithoutSource("logs_config.fingerprint_config.max_bytes", 256)
	mockConfig.SetWithoutSource("logs_config.fingerprint_config.max_lines", 1)
	mockConfig.SetWithoutSource("logs_config.fingerprint_config.to_skip", 0)

	sleepDuration := 20 * time.Millisecond
	fc := flareController.NewFlareController()
	s := NewLauncher(suite.openFilesLimit, sleepDuration, false, 10*time.Second, "by_name", fc, suite.tagger)
	s.pipelineProvider = suite.pipelineProvider
	s.registry = auditorMock.NewMockRegistry()
	s.activeSources = append(s.activeSources, suite.source)
	status.Clear()
	status.InitStatus(mockConfig, util.CreateSources([]*sources.LogSource{suite.source}))
	defer status.Clear()

	// Write initial content
	_, err := suite.testFile.WriteString("hello world\n")
	suite.Nil(err)
	suite.Nil(suite.testFile.Sync())

	s.scan()

	// Read message to confirm tailer is working
	msg := <-suite.outputChan
	suite.Equal("hello world", string(msg.GetContent()))

	// Get tailer and manually update fingerprint in registry
	tailer, found := s.tailers.Get(getScanKey(suite.testPath, suite.source))
	suite.True(found, "tailer should be found")

	// Create fingerprint config - use the same values as the mock config
	maxLines := 1
	maxBytes := 256 // Match the mock config value
	toSkip := 0
	fingerprintConfig := &config.FingerprintConfig{
		MaxLines: maxLines,
		MaxBytes: maxBytes,
		ToSkip:   toSkip,
	}
	filePath := tailer.Identifier()[5:]
	fingerprint := filetailer.ComputeFingerprint(filePath, fingerprintConfig)
	s.registry.(*auditorMock.Registry).SetFingerprint(fingerprint)
	s.registry.(*auditorMock.Registry).SetFingerprintConfig(fingerprintConfig)

	// Rotate file
	os.Rename(suite.testPath, suite.testRotatedPath)
	f, err := os.Create(suite.testPath)
	suite.Nil(err)

	// Write different content
	_, err = f.WriteString("hello again\n")
	suite.Nil(err)
	suite.Nil(f.Sync())
	defer f.Close()

	s.scan()

	newTailer, _ := s.tailers.Get(getScanKey(suite.testPath, suite.source))
	suite.True(tailer != newTailer, "A new tailer should have been created due to content change")
	filePath = newTailer.Identifier()[5:]
	newFingerprint := filetailer.ComputeFingerprint(filePath, fingerprintConfig)
	registryFingerprint := s.registry.GetFingerprint(newTailer.Identifier())
	suite.NotEqual(registryFingerprint, newFingerprint, "The fingerprint of the new file should be different")

	msg = <-suite.outputChan
	suite.Equal("hello again", string(msg.GetContent()))
}

func (suite *LauncherTestSuite) TestLauncherScanWithLogRotationAndChecksum_NoRotationOccurs() {
	suite.s.cleanup()
	mockConfig := configmock.New(suite.T())
	mockConfig.SetWithoutSource("logs_config.fingerprint_strategy", "checksum")
	mockConfig.SetWithoutSource("logs_config.fingerprint_config.max_bytes", 256)

	sleepDuration := 20 * time.Millisecond
	fc := flareController.NewFlareController()
	s := NewLauncher(suite.openFilesLimit, sleepDuration, false, 10*time.Second, "by_name", fc, suite.tagger)
	s.pipelineProvider = suite.pipelineProvider
	s.registry = auditorMock.NewMockRegistry()
	s.activeSources = append(s.activeSources, suite.source)
	status.Clear()
	status.InitStatus(mockConfig, util.CreateSources([]*sources.LogSource{suite.source}))
	defer status.Clear()

	// Write initial content
	initialContent := "hello world\n"
	_, err := suite.testFile.WriteString(initialContent)
	suite.Nil(err)
	suite.Nil(suite.testFile.Sync())

	s.scan()

	// Read message
	msg := <-suite.outputChan
	suite.Equal("hello world", string(msg.GetContent()))

	// Get tailer and verify it's working
	tailer, found := s.tailers.Get(getScanKey(suite.testPath, suite.source))
	suite.True(found, "tailer should be found")

	// Write more content to the same file (no rotation)
	additionalContent := "hello again\n"
	_, err = suite.testFile.WriteString(additionalContent)
	suite.Nil(err)
	suite.Nil(suite.testFile.Sync())

	// Verify rotation is NOT detected
	didRotate, err := tailer.DidRotateViaFingerprint()
	suite.Nil(err)
	suite.False(didRotate, "Should not detect rotation when writing to the same file")

	// Scan again - should not trigger any rotation logic
	s.scan()

	// Verify the same tailer is still being used
	newTailer, _ := s.tailers.Get(getScanKey(suite.testPath, suite.source))
	suite.True(tailer == newTailer, "The same tailer should continue as no rotation occurred")

	// Read the additional message
	msg = <-suite.outputChan
	suite.Equal("hello again", string(msg.GetContent()))
}

func (suite *LauncherTestSuite) TestLauncherScanWithLogRotationCopyTruncate() {
	s := suite.s
	var tailer *filetailer.Tailer
	var newTailer *filetailer.Tailer
	var err error
	var msg *message.Message

	tailer, _ = s.tailers.Get(getScanKey(suite.testPath, suite.source))
	_, err = suite.testFile.WriteString("hello world\n")
	suite.Nil(err)
	msg = <-suite.outputChan
	suite.Equal("hello world", string(msg.GetContent()))

	suite.Nil(suite.testFile.Truncate(0))
	_, err = suite.testFile.Seek(0, 0)
	suite.Nil(err)
	suite.Nil(suite.testFile.Sync())

	_, err = suite.testFile.WriteString("third\n")
	suite.Nil(err)

	suite.Nil(suite.testFile.Sync())
	s.scan()

	newTailer, _ = s.tailers.Get(getScanKey(suite.testPath, suite.source))
	suite.True(tailer != newTailer)

	msg = <-suite.outputChan
	suite.Equal("third", string(msg.GetContent()))
}

func (suite *LauncherTestSuite) TestLauncherScanWithFileRemovedAndCreated() {
	s := suite.s
	tailerLen := s.tailers.Count()

	var err error

	// remove file
	err = os.Remove(suite.testPath)
	suite.Nil(err)
	s.scan()
	suite.Equal(tailerLen-1, s.tailers.Count())

	// create file
	_, err = os.Create(suite.testPath)
	suite.Nil(err)
	s.scan()
	suite.Equal(tailerLen, s.tailers.Count())
}

func (suite *LauncherTestSuite) TestLifeCycle() {
	s := suite.s
	suite.Equal(1, s.tailers.Count())
	s.Start(launchers.NewMockSourceProvider(), suite.pipelineProvider, auditorMock.NewMockRegistry(), tailers.NewTailerTracker())

	// all tailers should be stopped
	s.Stop()
	suite.Equal(0, s.tailers.Count())
}

func TestLauncherTestSuite(t *testing.T) {
	suite.Run(t, new(LauncherTestSuite))
}

func TestLauncherTestSuiteWithConfigID(t *testing.T) {
	s := new(LauncherTestSuite)
	s.configID = "123456789"
	suite.Run(t, s)
}

func TestLauncherScanStartNewTailer(t *testing.T) {
	cfg := configmock.New(t)
	var path string
	var msg *message.Message
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	IDs := []string{"", "123456789"}

	for _, configID := range IDs {
		testDir := t.TempDir()

		// create launcher
		path = fmt.Sprintf("%s/*.log", testDir)
		openFilesLimit := 2
		sleepDuration := 20 * time.Millisecond
		fc := flareController.NewFlareController()
		launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second, "by_name", fc, fakeTagger)
		launcher.pipelineProvider = mock.NewMockProvider()
		launcher.registry = auditorMock.NewMockRegistry()
		outputChan := launcher.pipelineProvider.NextPipelineChan()
		source := sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Identifier: configID, Path: path})
		launcher.activeSources = append(launcher.activeSources, source)
		status.Clear()
		status.InitStatus(cfg, util.CreateSources([]*sources.LogSource{source}))
		defer status.Clear()

		// create file
		path = fmt.Sprintf("%s/test.log", testDir)
		file, err := os.Create(path)
		assert.Nil(t, err)

		// add content
		_, err = file.WriteString("hello\n")
		assert.Nil(t, err)
		_, err = file.WriteString("world\n")
		assert.Nil(t, err)
		file.Close()

		// test scan from beginning
		launcher.scan()
		assert.Equal(t, 1, launcher.tailers.Count())
		msg = <-outputChan
		assert.Equal(t, "hello", string(msg.GetContent()))
		msg = <-outputChan
		assert.Equal(t, "world", string(msg.GetContent()))
	}
}

func TestLauncherScanStartNewTailerForEmptyFile(t *testing.T) {
	mockConfig := configmock.New(t)

	// Temporarily set the global config for this test
	mockConfig.SetWithoutSource("logs_config.fingerprint_strategy", "checksum")
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	testDir := t.TempDir()

	// create launcher
	path := fmt.Sprintf("%s/*.log", testDir)
	openFilesLimit := 2
	sleepDuration := 20 * time.Millisecond
	fc := flareController.NewFlareController()
	launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second, "by_name", fc, fakeTagger)
	launcher.pipelineProvider = mock.NewMockProvider()
	launcher.registry = auditorMock.NewMockRegistry()
	source := sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: path})
	launcher.activeSources = append(launcher.activeSources, source)
	status.Clear()
	status.InitStatus(mockConfig, util.CreateSources([]*sources.LogSource{source}))
	defer status.Clear()

	// create empty file
	_, err := os.Create(fmt.Sprintf("%s/test.log", testDir))
	assert.Nil(t, err)

	launcher.scan()
	assert.Equal(t, 0, launcher.tailers.Count())
}

func TestLauncherScanStartNewTailerWithOneLine(t *testing.T) {
	mockConfig := configmock.New(t)

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	testDir := t.TempDir()

	// create launcher
	path := fmt.Sprintf("%s/*.log", testDir)
	openFilesLimit := 2
	sleepDuration := 20 * time.Millisecond
	fc := flareController.NewFlareController()
	launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second, "by_name", fc, fakeTagger)
	launcher.pipelineProvider = mock.NewMockProvider()
	launcher.registry = auditorMock.NewMockRegistry()
	source := sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: path, FingerprintConfig: config.FingerprintConfig{MaxLines: 1, MaxBytes: 256, ToSkip: 0}})
	launcher.activeSources = append(launcher.activeSources, source)
	status.Clear()
	status.InitStatus(mockConfig, util.CreateSources([]*sources.LogSource{source}))
	defer status.Clear()

	// create file
	filePath := fmt.Sprintf("%s/test.log", testDir)
	file, err := os.Create(filePath)
	assert.Nil(t, err)

	// add content
	_, err = file.WriteString("hello\n")
	assert.Nil(t, err)
	file.Close()

	// test scan from beginning
	launcher.scan()
	assert.Equal(t, 1, launcher.tailers.Count())
}

func TestLauncherScanStartNewTailerWithLongLine(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("logs_config.fingerprint_strategy", "checksum")
	mockConfig.SetWithoutSource("logs_config.fingerprint_config.max_bytes", 2048)
	// Temporarily set the global config for this test
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	testDir := t.TempDir()

	// create launcher
	path := fmt.Sprintf("%s/*.log", testDir)
	openFilesLimit := 2
	sleepDuration := 20 * time.Millisecond
	fc := flareController.NewFlareController()
	launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second, "by_name", fc, fakeTagger)
	launcher.pipelineProvider = mock.NewMockProvider()
	launcher.registry = auditorMock.NewMockRegistry()
	source := sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: path})
	launcher.activeSources = append(launcher.activeSources, source)
	status.Clear()
	status.InitStatus(mockConfig, util.CreateSources([]*sources.LogSource{source}))
	defer status.Clear()

	// create file
	filePath := fmt.Sprintf("%s/test.log", testDir)
	file, err := os.Create(filePath)
	assert.Nil(t, err)

	// add content
	longLine := strings.Repeat("a", 3000)
	_, err = file.WriteString(longLine + "\n")
	assert.Nil(t, err)
	file.Close()

	// test scan from beginning
	launcher.scan()
	assert.Equal(t, 1, launcher.tailers.Count())
}

func TestLauncherWithConcurrentContainerTailer(t *testing.T) {
	testDir := t.TempDir()
	path := fmt.Sprintf("%s/container.log", testDir)
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	// create launcher
	openFilesLimit := 3
	sleepDuration := 20 * time.Millisecond
	fc := flareController.NewFlareController()
	launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second, "by_name", fc, fakeTagger)
	launcher.pipelineProvider = mock.NewMockProvider()
	launcher.registry = auditorMock.NewMockRegistry()
	outputChan := launcher.pipelineProvider.NextPipelineChan()
	firstSource := sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: fmt.Sprintf("%s/*.log", testDir), TailingMode: "beginning", Identifier: "123456789"})
	secondSource := sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: fmt.Sprintf("%s/*.log", testDir), TailingMode: "beginning", Identifier: "987654321"})

	// create/truncate file
	file, err := os.Create(path)
	assert.Nil(t, err)

	// add content before starting the tailer
	_, err = file.WriteString("Once\n")
	assert.Nil(t, err)
	_, err = file.WriteString("Upon\n")
	assert.Nil(t, err)

	// test scan from the beginning, it shall read previously written strings
	launcher.addSource(firstSource)
	assert.Equal(t, 1, launcher.tailers.Count())

	// add content after starting the tailer
	_, err = file.WriteString("A\n")
	assert.Nil(t, err)
	_, err = file.WriteString("Time\n")
	assert.Nil(t, err)

	msg := <-outputChan
	assert.Equal(t, "Once", string(msg.GetContent()))
	msg = <-outputChan
	assert.Equal(t, "Upon", string(msg.GetContent()))
	msg = <-outputChan
	assert.Equal(t, "A", string(msg.GetContent()))
	msg = <-outputChan
	assert.Equal(t, "Time", string(msg.GetContent()))

	// Add a second source, same file, different container ID, tailing twice the same file is supported in that case
	launcher.addSource(secondSource)
	assert.Equal(t, 2, launcher.tailers.Count())
}

func TestLauncherTailFromTheBeginning(t *testing.T) {
	testDir := t.TempDir()
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	// create launcher
	openFilesLimit := 3
	sleepDuration := 20 * time.Millisecond
	fc := flareController.NewFlareController()
	launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second, "by_name", fc, fakeTagger)
	launcher.pipelineProvider = mock.NewMockProvider()
	launcher.registry = auditorMock.NewMockRegistry()
	outputChan := launcher.pipelineProvider.NextPipelineChan()
	sources := []*sources.LogSource{
		sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: fmt.Sprintf("%s/test.log", testDir), TailingMode: "beginning"}),
		sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: fmt.Sprintf("%s/container.log", testDir), TailingMode: "beginning", Identifier: "123456789"}),
		// Same file different container ID
		sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: fmt.Sprintf("%s/container.log", testDir), TailingMode: "beginning", Identifier: "987654321"}),
	}

	for i, source := range sources {
		// create/truncate file
		file, err := os.Create(source.Config.Path)
		assert.Nil(t, err)

		// add content before starting the tailer
		_, err = file.WriteString("Once\n")
		assert.Nil(t, err)
		_, err = file.WriteString("Upon\n")
		assert.Nil(t, err)

		// test scan from the beginning, it shall read previously written strings
		launcher.addSource(source)
		assert.Equal(t, i+1, launcher.tailers.Count())

		// add content after starting the tailer
		_, err = file.WriteString("A\n")
		assert.Nil(t, err)
		_, err = file.WriteString("Time\n")
		assert.Nil(t, err)

		msg := <-outputChan
		assert.Equal(t, "Once", string(msg.GetContent()))
		msg = <-outputChan
		assert.Equal(t, "Upon", string(msg.GetContent()))
		msg = <-outputChan
		assert.Equal(t, "A", string(msg.GetContent()))
		msg = <-outputChan
		assert.Equal(t, "Time", string(msg.GetContent()))
	}
}

func TestLauncherSetTail(t *testing.T) {
	testDir := t.TempDir()
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	path1 := fmt.Sprintf("%s/test.log", testDir)
	path2 := fmt.Sprintf("%s/test2.log", testDir)
	os.Create(path1)
	os.Create(path2)
	openFilesLimit := 2
	sleepDuration := 20 * time.Millisecond
	fc := flareController.NewFlareController()
	launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second, "by_name", fc, fakeTagger)
	launcher.pipelineProvider = mock.NewMockProvider()
	launcher.registry = auditorMock.NewMockRegistry()

	// Set tailing mode
	source := sources.NewLogSource("source1", &config.LogsConfig{Type: config.FileType, Path: path1, TailingMode: "end"})
	source2 := sources.NewLogSource("source2", &config.LogsConfig{Type: config.FileType, Path: path2, TailingMode: "beginning"})

	launcher.addSource(source)
	launcher.addSource(source2)
	tailer, _ := launcher.tailers.Get(getScanKey(path1, source))
	tailer2, _ := launcher.tailers.Get(getScanKey(path2, source2))
	assert.Equal(t, "end", tailer.Source().Config.TailingMode)
	assert.Equal(t, "beginning", tailer2.Source().Config.TailingMode)
}

func TestLauncherConfigIdentifier(t *testing.T) {
	testDir := t.TempDir()
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	path := fmt.Sprintf("%s/test.log", testDir)
	os.Create(path)
	openFilesLimit := 2
	sleepDuration := 20 * time.Millisecond
	fc := flareController.NewFlareController()
	launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second, "by_name", fc, fakeTagger)
	launcher.pipelineProvider = mock.NewMockProvider()
	launcher.registry = auditorMock.NewMockRegistry()

	// Set Identifier
	source := sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: path, Identifier: "NonEmptyString"})

	launcher.addSource(source)
	tailer, _ := launcher.tailers.Get(getScanKey(path, source))
	assert.Equal(t, "beginning", tailer.Source().Config.TailingMode)

}

func TestLauncherScanWithTooManyFiles(t *testing.T) {
	cfg := configmock.New(t)

	var err error
	var path string

	testDir := t.TempDir()
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	// creates files
	path = fmt.Sprintf("%s/1.log", testDir)
	_, err = os.Create(path)
	assert.Nil(t, err)

	path = fmt.Sprintf("%s/2.log", testDir)
	_, err = os.Create(path)
	assert.Nil(t, err)

	path = fmt.Sprintf("%s/3.log", testDir)
	_, err = os.Create(path)
	assert.Nil(t, err)

	// create launcher
	path = fmt.Sprintf("%s/*.log", testDir)
	openFilesLimit := 2
	sleepDuration := 20 * time.Millisecond
	fc := flareController.NewFlareController()
	launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second, "by_name", fc, fakeTagger)
	launcher.pipelineProvider = mock.NewMockProvider()
	launcher.registry = auditorMock.NewMockRegistry()
	source := sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: path})
	launcher.activeSources = append(launcher.activeSources, source)
	status.Clear()
	status.InitStatus(cfg, util.CreateSources([]*sources.LogSource{source}))
	defer status.Clear()

	// test at scan
	launcher.scan()
	assert.Equal(t, 2, launcher.tailers.Count())
	// Confirm that all of the files have been keepalive'd even if they are not tailed
	assert.Equal(t, 3, len(launcher.registry.(*auditorMock.Registry).KeepAlives))

	path = fmt.Sprintf("%s/2.log", testDir)
	err = os.Remove(path)
	assert.Nil(t, err)

	launcher.scan()
	assert.Equal(t, 2, launcher.tailers.Count())
}

func TestLauncherUpdatesSourceForExistingTailer(t *testing.T) {
	testDir := t.TempDir()
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	path := fmt.Sprintf("%s/*.log", testDir)
	os.Create(path)
	openFilesLimit := 2
	sleepDuration := 20 * time.Millisecond
	fc := flareController.NewFlareController()
	launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second, "by_name", fc, fakeTagger)
	launcher.pipelineProvider = mock.NewMockProvider()
	launcher.registry = auditorMock.NewMockRegistry()

	source := sources.NewLogSource("Source 1", &config.LogsConfig{Type: config.FileType, Identifier: "TEST_ID", Path: path})

	launcher.addSource(source)
	tailer, _ := launcher.tailers.Get(getScanKey(path, source))

	// test scan from beginning
	assert.Equal(t, 1, launcher.tailers.Count())
	assert.Equal(t, tailer.Source(), source)

	// Add a new source with the same file
	source2 := sources.NewLogSource("Source 2", &config.LogsConfig{Type: config.FileType, Identifier: "TEST_ID", Path: path})

	launcher.addSource(source2)

	// Source is replaced with the new source on the same tailer
	assert.Equal(t, tailer.Source(), source2)
}

func TestLauncherScanRecentFilesWithRemoval(t *testing.T) {
	cfg := configmock.New(t)

	var err error

	testDir := t.TempDir()
	baseTime := time.Date(2010, time.August, 10, 25, 0, 0, 0, time.UTC)
	openFilesLimit := 2

	path := func(name string) string {
		return fmt.Sprintf("%s/%s", testDir, name)
	}

	createFile := func(name string, time time.Time) {
		_, err = os.Create(path(name))
		assert.Nil(t, err)
		err = os.Chtimes(path(name), time, time)
		assert.Nil(t, err)
	}
	rmFile := func(name string) {
		err = os.Remove(path(name))
		assert.Nil(t, err)
	}
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	createLauncher := func() *Launcher {
		sleepDuration := 20 * time.Millisecond
		launcher := &Launcher{
			tailingLimit:           openFilesLimit,
			fileProvider:           fileprovider.NewFileProvider(openFilesLimit, fileprovider.WildcardUseFileModTime),
			tailers:                tailers.NewTailerContainer[*filetailer.Tailer](),
			tailerSleepDuration:    sleepDuration,
			stop:                   make(chan struct{}),
			validatePodContainerID: false,
			scanPeriod:             10 * time.Second,
			flarecontroller:        flareController.NewFlareController(),
			tagger:                 fakeTagger,
		}
		launcher.pipelineProvider = mock.NewMockProvider()
		launcher.registry = auditorMock.NewMockRegistry()
		logDirectory := fmt.Sprintf("%s/*.log", testDir)
		source := sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: logDirectory})
		launcher.activeSources = append(launcher.activeSources, source)
		status.Clear()
		status.InitStatus(cfg, util.CreateSources([]*sources.LogSource{source}))

		return launcher
	}

	// Given 4 files with descending mtimes
	createFile("1.log", baseTime.Add(time.Second*4))
	createFile("2.log", baseTime.Add(time.Second*3))
	createFile("3.log", baseTime.Add(time.Second*2))
	createFile("4.log", baseTime.Add(time.Second*1))
	launcher := createLauncher()
	defer status.Clear()

	launcher.scan()
	assert.Equal(t, 2, launcher.tailers.Count())
	assert.True(t, launcher.tailers.Contains(path("1.log")))
	assert.True(t, launcher.tailers.Contains(path("2.log")))

	// When ... the newest file gets rm'd
	rmFile("2.log")
	launcher.scan()

	// Then the next 2 most recently modified should be tailed
	assert.Equal(t, 2, launcher.tailers.Count())
	assert.True(t, launcher.tailers.Contains(path("1.log")))
	assert.True(t, launcher.tailers.Contains(path("3.log")))
}

func TestLauncherScanRecentFilesWithNewFiles(t *testing.T) {
	cfg := configmock.New(t)

	var err error

	testDir := t.TempDir()
	baseTime := time.Date(2010, time.August, 10, 25, 0, 0, 0, time.UTC)
	openFilesLimit := 2
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	path := func(name string) string {
		return fmt.Sprintf("%s/%s", testDir, name)
	}

	createFile := func(name string, time time.Time) {
		_, err = os.Create(path(name))
		assert.Nil(t, err)
		err = os.Chtimes(path(name), time, time)
		assert.Nil(t, err)
	}

	createLauncher := func() *Launcher {
		sleepDuration := 20 * time.Millisecond
		fc := flareController.NewFlareController()
		launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second, "by_modification_time", fc, fakeTagger)
		launcher.pipelineProvider = mock.NewMockProvider()
		launcher.registry = auditorMock.NewMockRegistry()
		logDirectory := fmt.Sprintf("%s/*.log", testDir)
		source := sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: logDirectory})
		launcher.activeSources = append(launcher.activeSources, source)
		status.Clear()
		status.InitStatus(cfg, util.CreateSources([]*sources.LogSource{source}))

		return launcher
	}

	// Given 4 files with descending mtimes
	createFile("1.log", baseTime.Add(time.Second*4))
	createFile("2.log", baseTime.Add(time.Second*3))
	createFile("3.log", baseTime.Add(time.Second*2))
	createFile("4.log", baseTime.Add(time.Second*1))
	launcher := createLauncher()
	defer status.Clear()

	launcher.scan()
	assert.Equal(t, 2, launcher.tailers.Count())
	assert.True(t, launcher.tailers.Contains(path("1.log")))
	assert.True(t, launcher.tailers.Contains(path("2.log")))

	// When ... a newer file appears
	createFile("7.log", baseTime.Add(time.Second*8))
	launcher.scan()

	// Then it should be tailed
	assert.Equal(t, 2, launcher.tailers.Count())
	assert.True(t, launcher.tailers.Contains(path("7.log")))
	assert.True(t, launcher.tailers.Contains(path("1.log")))

	// When ... an even newer file appears
	createFile("a.log", baseTime.Add(time.Second*10))
	launcher.scan()

	// Then it should be tailed
	assert.Equal(t, 2, launcher.tailers.Count())
	assert.True(t, launcher.tailers.Contains(path("7.log")))
	assert.True(t, launcher.tailers.Contains(path("a.log")))
}

func TestLauncherFileRotation(t *testing.T) {
	cfg := configmock.New(t)

	var err error

	testDir := t.TempDir()
	openFilesLimit := 2
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	path := func(name string) string {
		return fmt.Sprintf("%s/%s", testDir, name)
	}
	createFile := func(name string) {
		_, err = os.Create(path(name))
		assert.Nil(t, err)
	}

	createLauncher := func() *Launcher {
		sleepDuration := 20 * time.Millisecond
		fc := flareController.NewFlareController()
		launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second, "by_name", fc, fakeTagger)
		launcher.pipelineProvider = mock.NewMockProvider()
		launcher.registry = auditorMock.NewMockRegistry()
		logDirectory := fmt.Sprintf("%s/*.log", testDir)
		source := sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: logDirectory})
		launcher.activeSources = append(launcher.activeSources, source)
		status.Clear()
		status.InitStatus(cfg, util.CreateSources([]*sources.LogSource{source}))

		return launcher
	}

	createFile("a.log")
	createFile("b.log")
	createFile("c.log")
	createFile("d.log")
	launcher := createLauncher()
	defer status.Clear()

	launcher.scan()
	assert.Equal(t, 2, launcher.tailers.Count())
	assert.Equal(t, 0, len(launcher.rotatedTailers))
	assert.True(t, launcher.tailers.Contains(path("c.log")))
	assert.True(t, launcher.tailers.Contains(path("d.log")))

	cTailer, isPresent := launcher.tailers.Get(path("c.log"))
	assert.True(t, isPresent)

	// Do Rotation
	err = os.Rename(path("c.log"), path("c.log.1"))
	assert.Nil(t, err)
	createFile("c.log")

	didRotate, err := cTailer.DidRotate()
	assert.Nil(t, err)
	assert.True(t, didRotate)

	launcher.scan()
	assert.Equal(t, launcher.tailers.Count(), 2)
	assert.Equal(t, 1, len(launcher.rotatedTailers))
	assert.True(t, launcher.tailers.Contains(path("c.log")))
	assert.True(t, launcher.tailers.Contains(path("d.log")))

	launcher.cleanup() // Stop all the tailers
	assert.Equal(t, launcher.tailers.Count(), 0)
	assert.Equal(t, len(launcher.rotatedTailers), 0)
}

func TestLauncherFileDetectionSingleScan(t *testing.T) {
	cfg := configmock.New(t)

	var err error

	testDir := t.TempDir()
	openFilesLimit := 2
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	path := func(name string) string {
		return fmt.Sprintf("%s/%s", testDir, name)
	}
	createFile := func(name string) {
		_, err = os.Create(path(name))
		assert.Nil(t, err)
	}

	createLauncher := func() *Launcher {
		sleepDuration := 20 * time.Millisecond
		fc := flareController.NewFlareController()
		launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second, "by_name", fc, fakeTagger)
		launcher.pipelineProvider = mock.NewMockProvider()
		launcher.registry = auditorMock.NewMockRegistry()
		logDirectory := fmt.Sprintf("%s/*.log", testDir)
		source := sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: logDirectory})
		launcher.activeSources = append(launcher.activeSources, source)
		status.Clear()
		status.InitStatus(cfg, util.CreateSources([]*sources.LogSource{source}))

		return launcher
	}

	createFile("a.log")
	createFile("b.log")
	launcher := createLauncher()
	defer status.Clear()

	launcher.scan()
	assert.Equal(t, 2, launcher.tailers.Count())
	assert.True(t, launcher.tailers.Contains(path("a.log")))
	assert.True(t, launcher.tailers.Contains(path("b.log")))

	createFile("z.log")

	launcher.scan()
	assert.Equal(t, launcher.tailers.Count(), 2)
	assert.True(t, launcher.tailers.Contains(path("z.log")))
	assert.True(t, launcher.tailers.Contains(path("b.log")))
}

func (suite *LauncherTestSuite) TestLauncherDoesNotCreateTailerForTruncatedUndersizedFile() {
	suite.s.cleanup()
	mockConfig := configmock.New(suite.T())
	mockConfig.SetWithoutSource("logs_config.fingerprint_strategy", "checksum")

	sleepDuration := 20 * time.Millisecond
	fc := flareController.NewFlareController()
	s := NewLauncher(suite.openFilesLimit, sleepDuration, false, 10*time.Second, "checksum", fc, suite.tagger)
	s.pipelineProvider = suite.pipelineProvider
	s.registry = auditorMock.NewMockRegistry()
	s.activeSources = append(s.activeSources, suite.source)
	status.Clear()
	status.InitStatus(mockConfig, util.CreateSources([]*sources.LogSource{suite.source}))
	defer status.Clear()

	// Write initial content
	_, err := suite.testFile.WriteString("hello world\n")
	suite.Nil(err)
	suite.Nil(suite.testFile.Sync())

	s.scan()

	// Read message to confirm tailer is working
	msg := <-suite.outputChan
	suite.Equal("hello world", string(msg.GetContent()))

	// Get initial tailer and verify rotation detection works
	tailer, found := s.tailers.Get(getScanKey(suite.testPath, suite.source))
	suite.True(found, "tailer should be found")
	initialTailerCount := s.tailers.Count()

	// Simulate rotation: truncate file to empty (fingerprint becomes 0)
	suite.Nil(suite.testFile.Truncate(0))
	_, err = suite.testFile.Seek(0, 0)
	suite.Nil(err)
	suite.Nil(suite.testFile.Sync())

	// Verify rotation is detected
	didRotate, err := tailer.DidRotateViaFingerprint()
	suite.Nil(err)
	suite.True(didRotate, "Should detect rotation when file becomes empty (fingerprint = 0)")

	// Now test the launcher's behavior: it should NOT create a new tailer for the undersized file
	s.scan()

	// Verify no new tailer was created for the undersized file
	// The old tailer should be removed but no new one should be created
	afterScanCount := s.tailers.Count()
	suite.Equal(initialTailerCount-1, afterScanCount, "No new tailer should be created for undersized file after rotation")
}

func (suite *LauncherTestSuite) TestLauncherDoesNotCreateTailerForRotatedUndersizedFile() {
	suite.s.cleanup()
	mockConfig := configmock.New(suite.T())
	mockConfig.SetWithoutSource("logs_config.fingerprint_strategy", "checksum")

	sleepDuration := 20 * time.Millisecond
	fc := flareController.NewFlareController()
	s := NewLauncher(suite.openFilesLimit, sleepDuration, false, 10*time.Second, "checksum", fc, suite.tagger)
	s.pipelineProvider = suite.pipelineProvider
	s.registry = auditorMock.NewMockRegistry()
	s.activeSources = append(s.activeSources, suite.source)
	status.Clear()
	status.InitStatus(mockConfig, util.CreateSources([]*sources.LogSource{suite.source}))
	defer status.Clear()

	// Write initial content
	_, err := suite.testFile.WriteString("hello world\n")
	suite.Nil(err)
	suite.Nil(suite.testFile.Sync())

	s.scan()

	// Read message to confirm tailer is working
	msg := <-suite.outputChan
	suite.Equal("hello world", string(msg.GetContent()))

	// Get initial tailer and verify rotation detection works
	tailer, found := s.tailers.Get(getScanKey(suite.testPath, suite.source))
	suite.True(found, "tailer should be found")
	initialTailerCount := s.tailers.Count()

	// Simulate file rotation: move current file to .1 and create a new empty file
	rotatedPath := suite.testPath + ".1"
	err = os.Rename(suite.testPath, rotatedPath)
	suite.Nil(err)

	// Create a new file that is undersized (empty, which results in fingerprint = 0)
	newFile, err := os.Create(suite.testPath)
	suite.Nil(err)
	newFile.Close()

	// Verify rotation is detected
	didRotate, err := tailer.DidRotateViaFingerprint()
	suite.Nil(err)
	suite.True(didRotate, "Should detect rotation when original file is moved and new file is created")

	// Now test the launcher's behavior: it should NOT create a new tailer for the undersized file
	s.scan()

	// Verify no new tailer was created for the undersized file
	// The old tailer should be removed but no new one should be created
	afterScanCount := s.tailers.Count() + len(s.rotatedTailers)
	suite.Equal(initialTailerCount, afterScanCount, "No new tailer should be created for undersized file after rotation")

	// Clean up the rotated file
	os.Remove(rotatedPath)
}

func getScanKey(path string, source *sources.LogSource) string {
	return filetailer.NewFile(path, source, false).GetScanKey()
}
