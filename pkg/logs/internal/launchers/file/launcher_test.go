// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package file

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	auditor "github.com/DataDog/datadog-agent/pkg/logs/auditor/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers"
	fileprovider "github.com/DataDog/datadog-agent/pkg/logs/internal/launchers/file/provider"
	filetailer "github.com/DataDog/datadog-agent/pkg/logs/internal/tailers/file"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/internal/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
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
	source           *sources.LogSource
	openFilesLimit   int
	s                *Launcher
}

func (suite *LauncherTestSuite) SetupTest() {
	suite.pipelineProvider = mock.NewMockProvider()
	suite.outputChan = suite.pipelineProvider.NextPipelineChan()

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
	suite.s = NewLauncher(suite.openFilesLimit, sleepDuration, false, 10*time.Second, "by_name")
	suite.s.pipelineProvider = suite.pipelineProvider
	suite.s.registry = auditor.NewRegistry()
	suite.s.activeSources = append(suite.s.activeSources, suite.source)
	status.InitStatus(util.CreateSources([]*sources.LogSource{suite.source}))
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

	suite.Nil(suite.testFile.Truncate(0))
	_, err = suite.testFile.Seek(0, 0)
	suite.Nil(err)
	suite.Nil(suite.testFile.Sync())

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
	var msg *message.Message

	IDs := []string{"", "123456789"}

	for _, configID := range IDs {
		testDir := t.TempDir()

		// create launcher
		path = fmt.Sprintf("%s/*.log", testDir)
		openFilesLimit := 2
		sleepDuration := 20 * time.Millisecond
		launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second, "by_name")
		launcher.pipelineProvider = mock.NewMockProvider()
		launcher.registry = auditor.NewRegistry()
		outputChan := launcher.pipelineProvider.NextPipelineChan()
		source := sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Identifier: configID, Path: path})
		launcher.activeSources = append(launcher.activeSources, source)
		status.Clear()
		status.InitStatus(util.CreateSources([]*sources.LogSource{source}))
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
	testDir := t.TempDir()
	path := fmt.Sprintf("%s/container.log", testDir)

	// create launcher
	openFilesLimit := 3
	sleepDuration := 20 * time.Millisecond
	launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second, "by_name")
	launcher.pipelineProvider = mock.NewMockProvider()
	launcher.registry = auditor.NewRegistry()
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
	testDir := t.TempDir()

	// create launcher
	openFilesLimit := 3
	sleepDuration := 20 * time.Millisecond
	launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second, "by_name")
	launcher.pipelineProvider = mock.NewMockProvider()
	launcher.registry = auditor.NewRegistry()
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

	testDir := t.TempDir()

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
	launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second, "by_name")
	launcher.pipelineProvider = mock.NewMockProvider()
	launcher.registry = auditor.NewRegistry()
	source := sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: path})
	launcher.activeSources = append(launcher.activeSources, source)
	status.Clear()
	status.InitStatus(util.CreateSources([]*sources.LogSource{source}))
	defer status.Clear()

	// test at scan
	launcher.scan()
	assert.Equal(t, 2, len(launcher.tailers))

	path = fmt.Sprintf("%s/2.log", testDir)
	err = os.Remove(path)
	assert.Nil(t, err)

	launcher.scan()
	assert.Equal(t, 2, len(launcher.tailers))
}

func TestLauncherUpdatesSourceForExistingTailer(t *testing.T) {

	testDir := t.TempDir()

	path := fmt.Sprintf("%s/*.log", testDir)
	os.Create(path)
	openFilesLimit := 2
	sleepDuration := 20 * time.Millisecond
	launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second, "by_name")
	launcher.pipelineProvider = mock.NewMockProvider()
	launcher.registry = auditor.NewRegistry()

	source := sources.NewLogSource("Source 1", &config.LogsConfig{Type: config.FileType, Identifier: "TEST_ID", Path: path})

	launcher.addSource(source)
	tailer := launcher.tailers[getScanKey(path, source)]

	// test scan from beginning
	assert.Equal(t, 1, len(launcher.tailers))
	assert.Equal(t, tailer.Source(), source)

	// Add a new source with the same file
	source2 := sources.NewLogSource("Source 2", &config.LogsConfig{Type: config.FileType, Identifier: "TEST_ID", Path: path})

	launcher.addSource(source2)

	// Source is replaced with the new source on the same tailer
	assert.Equal(t, tailer.Source(), source2)
}

func TestLauncherScanRecentFilesWithRemoval(t *testing.T) {
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

	createLauncher := func() *Launcher {
		sleepDuration := 20 * time.Millisecond
		launcher := &Launcher{
			tailingLimit:           openFilesLimit,
			fileProvider:           fileprovider.NewFileProvider(openFilesLimit, fileprovider.WildcardUseFileModTime),
			tailers:                make(map[string]*tailer.Tailer),
			tailerSleepDuration:    sleepDuration,
			stop:                   make(chan struct{}),
			validatePodContainerID: false,
			scanPeriod:             10 * time.Second,
		}
		launcher.pipelineProvider = mock.NewMockProvider()
		launcher.registry = auditor.NewRegistry()
		logDirectory := fmt.Sprintf("%s/*.log", testDir)
		source := sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: logDirectory})
		launcher.activeSources = append(launcher.activeSources, source)
		status.Clear()
		status.InitStatus(util.CreateSources([]*sources.LogSource{source}))

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
	assert.Equal(t, 2, len(launcher.tailers))
	assert.Contains(t, launcher.tailers, path("1.log"))
	assert.Contains(t, launcher.tailers, path("2.log"))

	// When ... the newest file gets rm'd
	rmFile("2.log")
	launcher.scan()

	// Then the next 2 most recently modified should be tailed
	assert.Equal(t, 2, len(launcher.tailers))
	assert.Contains(t, launcher.tailers, path("1.log"))
	assert.Contains(t, launcher.tailers, path("3.log"))
}

func TestLauncherScanRecentFilesWithNewFiles(t *testing.T) {
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

	createLauncher := func() *Launcher {
		sleepDuration := 20 * time.Millisecond
		launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second, "by_modification_time")
		launcher.pipelineProvider = mock.NewMockProvider()
		launcher.registry = auditor.NewRegistry()
		logDirectory := fmt.Sprintf("%s/*.log", testDir)
		source := sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: logDirectory})
		launcher.activeSources = append(launcher.activeSources, source)
		status.Clear()
		status.InitStatus(util.CreateSources([]*sources.LogSource{source}))

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
	assert.Equal(t, 2, len(launcher.tailers))
	assert.Contains(t, launcher.tailers, path("1.log"))
	assert.Contains(t, launcher.tailers, path("2.log"))

	// When ... a newer file appears
	createFile("7.log", baseTime.Add(time.Second*8))
	launcher.scan()

	// Then it should be tailed
	assert.Equal(t, 2, len(launcher.tailers))
	assert.Contains(t, launcher.tailers, path("7.log"))
	assert.Contains(t, launcher.tailers, path("1.log"))

	// When ... an even newer file appears
	createFile("a.log", baseTime.Add(time.Second*10))
	launcher.scan()

	// Then it should be tailed
	assert.Equal(t, 2, len(launcher.tailers))
	assert.Contains(t, launcher.tailers, path("7.log"))
	assert.Contains(t, launcher.tailers, path("a.log"))
}

func TestLauncherFileRotation(t *testing.T) {
	var err error

	testDir := t.TempDir()
	openFilesLimit := 2

	path := func(name string) string {
		return fmt.Sprintf("%s/%s", testDir, name)
	}
	createFile := func(name string) {
		_, err = os.Create(path(name))
		assert.Nil(t, err)
	}

	createLauncher := func() *Launcher {
		sleepDuration := 20 * time.Millisecond
		launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second, "by_name")
		launcher.pipelineProvider = mock.NewMockProvider()
		launcher.registry = auditor.NewRegistry()
		logDirectory := fmt.Sprintf("%s/*.log", testDir)
		source := sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: logDirectory})
		launcher.activeSources = append(launcher.activeSources, source)
		status.Clear()
		status.InitStatus(util.CreateSources([]*sources.LogSource{source}))

		return launcher
	}

	createFile("a.log")
	createFile("b.log")
	createFile("c.log")
	createFile("d.log")
	launcher := createLauncher()
	defer status.Clear()

	launcher.scan()
	assert.Equal(t, 2, len(launcher.tailers))
	assert.Contains(t, launcher.tailers, path("c.log"))
	assert.Contains(t, launcher.tailers, path("d.log"))

	cTailer, isPresent := launcher.tailers[path("c.log")]
	assert.True(t, isPresent)

	// Do Rotation
	err = os.Rename(path("c.log"), path("c.log.1"))
	assert.Nil(t, err)
	createFile("c.log")

	didRotate, err := cTailer.DidRotate()
	assert.Nil(t, err)
	assert.True(t, didRotate)

	launcher.scan()
	assert.Len(t, launcher.tailers, 2)
	assert.Contains(t, launcher.tailers, path("c.log"))
	assert.Contains(t, launcher.tailers, path("d.log"))
}

func TestLauncherFileDetectionSingleScan(t *testing.T) {
	var err error

	testDir := t.TempDir()
	openFilesLimit := 2

	path := func(name string) string {
		return fmt.Sprintf("%s/%s", testDir, name)
	}
	createFile := func(name string) {
		_, err = os.Create(path(name))
		assert.Nil(t, err)
	}

	createLauncher := func() *Launcher {
		sleepDuration := 20 * time.Millisecond
		launcher := NewLauncher(openFilesLimit, sleepDuration, false, 10*time.Second, "by_name")
		launcher.pipelineProvider = mock.NewMockProvider()
		launcher.registry = auditor.NewRegistry()
		logDirectory := fmt.Sprintf("%s/*.log", testDir)
		source := sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: logDirectory})
		launcher.activeSources = append(launcher.activeSources, source)
		status.Clear()
		status.InitStatus(util.CreateSources([]*sources.LogSource{source}))

		return launcher
	}

	createFile("a.log")
	createFile("b.log")
	launcher := createLauncher()
	defer status.Clear()

	launcher.scan()
	assert.Equal(t, 2, len(launcher.tailers))
	assert.Contains(t, launcher.tailers, path("a.log"))
	assert.Contains(t, launcher.tailers, path("b.log"))

	createFile("z.log")

	launcher.scan()
	assert.Len(t, launcher.tailers, 2)
	assert.Contains(t, launcher.tailers, path("z.log"))
	assert.Contains(t, launcher.tailers, path("b.log"))
}

func getScanKey(path string, source *sources.LogSource) string {
	return filetailer.NewFile(path, source, false).GetScanKey()
}
