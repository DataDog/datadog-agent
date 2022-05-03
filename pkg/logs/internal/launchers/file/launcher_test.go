// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package file

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	auditor "github.com/DataDog/datadog-agent/pkg/logs/auditor/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers"
	filetailer "github.com/DataDog/datadog-agent/pkg/logs/internal/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
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
	source           *config.LogSource
	openFilesLimit   int
	s                *Launcher
}

func (suite *LauncherTestSuite) SetupTest() {
	suite.pipelineProvider = mock.NewMockProvider()
	suite.outputChan = suite.pipelineProvider.NextPipelineChan()

	var err error
	suite.testDir, err = ioutil.TempDir("", "log-launcher-test-")
	suite.Nil(err)

	suite.testPath = fmt.Sprintf("%s/launcher.log", suite.testDir)
	suite.testRotatedPath = fmt.Sprintf("%s.1", suite.testPath)

	f, err := os.Create(suite.testPath)
	suite.Nil(err)
	suite.testFile = f
	f, err = os.Create(suite.testRotatedPath)
	suite.Nil(err)
	suite.testRotatedFile = f

	suite.openFilesLimit = 100
	suite.source = config.NewLogSource("", &config.LogsConfig{Type: config.FileType, Identifier: suite.configID, Path: suite.testPath})
	sleepDuration := 20 * time.Millisecond
	suite.s = NewLauncher(suite.openFilesLimit, sleepDuration, false, 10*time.Second)
	suite.s.pipelineProvider = suite.pipelineProvider
	suite.s.registry = auditor.NewRegistry()
	suite.s.activeSources = append(suite.s.activeSources, suite.source)
	status.InitStatus(config.CreateSources([]*config.LogSource{suite.source}))
	suite.s.scan()
}

func (suite *LauncherTestSuite) TearDownTest() {
	status.Clear()
	suite.testFile.Close()
	suite.testRotatedFile.Close()
	os.Remove(suite.testDir)
	suite.s.cleanup()
}

func (suite *LauncherTestSuite) TestLauncherStartsTailers() {
	_, err := suite.testFile.WriteString("hello world\n")
	suite.Nil(err)
	msg := <-suite.outputChan
	suite.Equal("hello world", string(msg.Content))
}

func (suite *LauncherTestSuite) TestLauncherScanWithoutLogRotation() {
	s := suite.s

	var tailer *filetailer.Tailer
	var newTailer *filetailer.Tailer
	var err error
	var msg *message.Message

	tailer = s.tailers[getScanKey(suite.testPath, suite.source)]
	_, err = suite.testFile.WriteString("hello world\n")
	suite.Nil(err)
	msg = <-suite.outputChan
	suite.Equal("hello world", string(msg.Content))

	s.scan()
	newTailer = s.tailers[getScanKey(suite.testPath, suite.source)]
	// testing that launcher did not have to create a new tailer
	suite.True(tailer == newTailer)

	_, err = suite.testFile.WriteString("hello again\n")
	suite.Nil(err)
	msg = <-suite.outputChan
	suite.Equal("hello again", string(msg.Content))
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
	suite.Equal("hello world", string(msg.Content))

	tailer = s.tailers[getScanKey(suite.testPath, suite.source)]
	os.Rename(suite.testPath, suite.testRotatedPath)
	f, err := os.Create(suite.testPath)
	suite.Nil(err)
	s.scan()
	newTailer = s.tailers[getScanKey(suite.testPath, suite.source)]
	suite.True(tailer != newTailer)

	_, err = f.WriteString("hello again\n")
	suite.Nil(err)
	msg = <-suite.outputChan
	suite.Equal("hello again", string(msg.Content))
}

func (suite *LauncherTestSuite) TestLauncherScanWithLogRotationCopyTruncate() {
	s := suite.s
	var tailer *filetailer.Tailer
	var newTailer *filetailer.Tailer
	var err error
	var msg *message.Message

	tailer = s.tailers[getScanKey(suite.testPath, suite.source)]
	_, err = suite.testFile.WriteString("hello world\n")
	suite.Nil(err)
	msg = <-suite.outputChan
	suite.Equal("hello world", string(msg.Content))

	suite.testFile.Truncate(0)
	suite.testFile.Seek(0, 0)
	suite.testFile.Sync()
	_, err = suite.testFile.WriteString("third\n")
	suite.Nil(err)

	s.scan()
	newTailer = s.tailers[getScanKey(suite.testPath, suite.source)]
	suite.True(tailer != newTailer)

	msg = <-suite.outputChan
	suite.Equal("third", string(msg.Content))
}

func (suite *LauncherTestSuite) TestLauncherScanWithFileRemovedAndCreated() {
	s := suite.s
	tailerLen := len(s.tailers)

	var err error

	// remove file
	err = os.Remove(suite.testPath)
	suite.Nil(err)
	s.scan()
	suite.Equal(tailerLen-1, len(s.tailers))

	// create file
	_, err = os.Create(suite.testPath)
	suite.Nil(err)
	s.scan()
	suite.Equal(tailerLen, len(s.tailers))
}

func (suite *LauncherTestSuite) TestLifeCycle() {
	s := suite.s
	suite.Equal(1, len(s.tailers))
	s.Start(launchers.NewMockSourceProvider(), suite.pipelineProvider, auditor.NewRegistry())

	// all tailers should be stopped
	s.Stop()
	suite.Equal(0, len(s.tailers))
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
	var path string
	var file *os.File
	var msg *message.Message

	IDs := []string{"", "123456789"}

	for _, configID := range IDs {
		testDir, err := ioutil.TempDir("", "log-launcher-test-")
		assert.Nil(t, err)

		// create launcher
		path = fmt.Sprintf("%s/*.log", testDir)
		openFilesLimit := 2
		sleepDuration := 20 * time.Millisecond
		launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second)
		launcher.pipelineProvider = mock.NewMockProvider()
		launcher.registry = auditor.NewRegistry()
		outputChan := launcher.pipelineProvider.NextPipelineChan()
		source := config.NewLogSource("", &config.LogsConfig{Type: config.FileType, Identifier: configID, Path: path})
		launcher.activeSources = append(launcher.activeSources, source)
		status.Clear()
		status.InitStatus(config.CreateSources([]*config.LogSource{source}))
		defer status.Clear()

		// create file
		path = fmt.Sprintf("%s/test.log", testDir)
		file, err = os.Create(path)
		assert.Nil(t, err)

		// add content
		_, err = file.WriteString("hello\n")
		assert.Nil(t, err)
		_, err = file.WriteString("world\n")
		assert.Nil(t, err)

		// test scan from beginning
		launcher.scan()
		assert.Equal(t, 1, len(launcher.tailers))
		msg = <-outputChan
		assert.Equal(t, "hello", string(msg.Content))
		msg = <-outputChan
		assert.Equal(t, "world", string(msg.Content))
	}
}

func TestLauncherWithConcurrentContainerTailer(t *testing.T) {
	testDir, err := ioutil.TempDir("", "log-launcher-test-")
	assert.Nil(t, err)
	path := fmt.Sprintf("%s/container.log", testDir)

	// create launcher
	openFilesLimit := 3
	sleepDuration := 20 * time.Millisecond
	launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second)
	launcher.pipelineProvider = mock.NewMockProvider()
	launcher.registry = auditor.NewRegistry()
	outputChan := launcher.pipelineProvider.NextPipelineChan()
	firstSource := config.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: fmt.Sprintf("%s/*.log", testDir), TailingMode: "beginning", Identifier: "123456789"})
	secondSource := config.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: fmt.Sprintf("%s/*.log", testDir), TailingMode: "beginning", Identifier: "987654321"})

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
	assert.Equal(t, 1, len(launcher.tailers))

	// add content after starting the tailer
	_, err = file.WriteString("A\n")
	assert.Nil(t, err)
	_, err = file.WriteString("Time\n")
	assert.Nil(t, err)

	msg := <-outputChan
	assert.Equal(t, "Once", string(msg.Content))
	msg = <-outputChan
	assert.Equal(t, "Upon", string(msg.Content))
	msg = <-outputChan
	assert.Equal(t, "A", string(msg.Content))
	msg = <-outputChan
	assert.Equal(t, "Time", string(msg.Content))

	// Add a second source, same file, different container ID, tailing twice the same file is supported in that case
	launcher.addSource(secondSource)
	assert.Equal(t, 2, len(launcher.tailers))
}

func TestLauncherTailFromTheBeginning(t *testing.T) {
	testDir, err := ioutil.TempDir("", "log-launcher-test-")
	assert.Nil(t, err)

	// create launcher
	openFilesLimit := 3
	sleepDuration := 20 * time.Millisecond
	launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second)
	launcher.pipelineProvider = mock.NewMockProvider()
	launcher.registry = auditor.NewRegistry()
	outputChan := launcher.pipelineProvider.NextPipelineChan()
	sources := []*config.LogSource{
		config.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: fmt.Sprintf("%s/test.log", testDir), TailingMode: "beginning"}),
		config.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: fmt.Sprintf("%s/container.log", testDir), TailingMode: "beginning", Identifier: "123456789"}),
		// Same file different container ID
		config.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: fmt.Sprintf("%s/container.log", testDir), TailingMode: "beginning", Identifier: "987654321"}),
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
		assert.Equal(t, i+1, len(launcher.tailers))

		// add content after starting the tailer
		_, err = file.WriteString("A\n")
		assert.Nil(t, err)
		_, err = file.WriteString("Time\n")
		assert.Nil(t, err)

		msg := <-outputChan
		assert.Equal(t, "Once", string(msg.Content))
		msg = <-outputChan
		assert.Equal(t, "Upon", string(msg.Content))
		msg = <-outputChan
		assert.Equal(t, "A", string(msg.Content))
		msg = <-outputChan
		assert.Equal(t, "Time", string(msg.Content))
	}
}

func TestLauncherScanWithTooManyFiles(t *testing.T) {
	var err error
	var path string

	testDir, err := ioutil.TempDir("", "log-launcher-test-")
	assert.Nil(t, err)

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
	launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second)
	launcher.pipelineProvider = mock.NewMockProvider()
	launcher.registry = auditor.NewRegistry()
	source := config.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: path})
	launcher.activeSources = append(launcher.activeSources, source)
	status.Clear()
	status.InitStatus(config.CreateSources([]*config.LogSource{source}))
	defer status.Clear()

	// test at scan
	launcher.scan()
	assert.Equal(t, 2, len(launcher.tailers))

	path = fmt.Sprintf("%s/2.log", testDir)
	err = os.Remove(path)
	assert.Nil(t, err)

	launcher.scan()
	assert.Equal(t, 1, len(launcher.tailers))

	launcher.scan()
	assert.Equal(t, 2, len(launcher.tailers))
}

func TestContainerIDInContainerLogFile(t *testing.T) {
	assert := assert.New(t)
	//func (s *Launcher) shouldIgnore(file *File) bool {
	logSource := config.NewLogSource("mylogsource", nil)
	logSource.SetSourceType(config.DockerSourceType)
	logSource.Config = &config.LogsConfig{
		Type: config.FileType,
		Path: "/var/log/pods/file-uuid-foo-bar.log",

		Identifier: "abcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcd",
	}

	// create an empty file that will represent the log file that would have been found in /var/log/containers
	ContainersLogsDir = "/tmp/"
	os.Remove("/tmp/myapp_my-namespace_myapp-abcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcd.log")

	err := os.Symlink("/var/log/pods/file-uuid-foo-bar.log", "/tmp/myapp_my-namespace_myapp-abcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcd.log")
	defer func() {
		// cleaning up after the test run
		os.Remove("/tmp/myapp_my-namespace_myapp-abcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcd.log")
		os.Remove("/tmp/myapp_my-namespace_myapp-thisisnotacontainerIDevenifthisispointingtothecorrectfile.log")
	}()

	assert.NoError(err, "error while creating the temporary file")

	file := filetailer.File{
		Path:           "/var/log/pods/file-uuid-foo-bar.log",
		IsWildcardPath: false,
		Source:         logSource,
	}

	launcher := &Launcher{}

	// we've found a symlink validating that the file we have just scanned is concerning the container we're currently processing for this source
	assert.False(launcher.shouldIgnore(&file), "the file existing in ContainersLogsDir is pointing to the same container, scanned file should be tailed")

	// now, let's change the container for which we are trying to scan files,
	// because the symlink is pointing from another container, we should ignore
	// that log file
	file.Source.Config.Identifier = "1234123412341234123412341234123412341234123412341234123412341234"
	assert.True(launcher.shouldIgnore(&file), "the file existing in ContainersLogsDir is not pointing to the same container, scanned file should be ignored")

	// in this scenario, no link is found in /var/log/containers, thus, we should not ignore the file
	os.Remove("/tmp/myapp_my-namespace_myapp-abcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcd.log")
	assert.False(launcher.shouldIgnore(&file), "no files existing in ContainersLogsDir, we should not ignore the file we have just scanned")

	// in this scenario, the file we've found doesn't look like a container ID
	os.Symlink("/var/log/pods/file-uuid-foo-bar.log", "/tmp/myapp_my-namespace_myapp-thisisnotacontainerIDevenifthisispointingtothecorrectfile.log")
	assert.False(launcher.shouldIgnore(&file), "no container ID found, we don't want to ignore this scanned file")
}

func getScanKey(path string, source *config.LogSource) string {
	return filetailer.NewFile(path, source, false).GetScanKey()
}
