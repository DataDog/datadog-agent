// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

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
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
)

type ScannerTestSuite struct {
	suite.Suite
	testDir         string
	testPath        string
	testFile        *os.File
	testRotatedPath string
	testRotatedFile *os.File

	outputChan       chan *message.Message
	pipelineProvider pipeline.Provider
	source           *config.LogSource
	openFilesLimit   int
	s                *Scanner
}

func (suite *ScannerTestSuite) SetupTest() {
	suite.pipelineProvider = mock.NewMockProvider()
	suite.outputChan = suite.pipelineProvider.NextPipelineChan()

	var err error
	suite.testDir, err = ioutil.TempDir("", "log-scanner-test-")
	suite.Nil(err)

	suite.testPath = fmt.Sprintf("%s/scanner.log", suite.testDir)
	suite.testRotatedPath = fmt.Sprintf("%s.1", suite.testPath)

	f, err := os.Create(suite.testPath)
	suite.Nil(err)
	suite.testFile = f
	f, err = os.Create(suite.testRotatedPath)
	suite.Nil(err)
	suite.testRotatedFile = f

	suite.openFilesLimit = 100
	suite.source = config.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: suite.testPath})
	sleepDuration := 20 * time.Millisecond
	suite.s = NewScanner(config.NewLogSources(), suite.openFilesLimit, suite.pipelineProvider, auditor.NewRegistry(), sleepDuration)
	suite.s.activeSources = append(suite.s.activeSources, suite.source)
	status.InitStatus(config.CreateSources([]*config.LogSource{suite.source}))
	suite.s.scan()
}

func (suite *ScannerTestSuite) TearDownTest() {
	status.Clear()
	suite.testFile.Close()
	suite.testRotatedFile.Close()
	os.Remove(suite.testDir)
	suite.s.cleanup()
}

func (suite *ScannerTestSuite) TestScannerStartsTailers() {
	_, err := suite.testFile.WriteString("hello world\n")
	suite.Nil(err)
	msg := <-suite.outputChan
	suite.Equal("hello world", string(msg.Content))
}

func (suite *ScannerTestSuite) TestScannerScanWithoutLogRotation() {
	s := suite.s
	source := suite.source

	var tailer *Tailer
	var newTailer *Tailer
	var err error
	var msg *message.Message

	tailer = s.tailers[source.Config.Path]
	_, err = suite.testFile.WriteString("hello world\n")
	suite.Nil(err)
	msg = <-suite.outputChan
	suite.Equal("hello world", string(msg.Content))

	s.scan()
	newTailer = s.tailers[source.Config.Path]
	// testing that scanner did not have to create a new tailer
	suite.True(tailer == newTailer)

	_, err = suite.testFile.WriteString("hello again\n")
	suite.Nil(err)
	msg = <-suite.outputChan
	suite.Equal("hello again", string(msg.Content))
}

func (suite *ScannerTestSuite) TestScannerScanWithLogRotation() {
	s := suite.s
	source := suite.source

	var tailer *Tailer
	var newTailer *Tailer
	var err error
	var msg *message.Message

	_, err = suite.testFile.WriteString("hello world\n")
	suite.Nil(err)
	msg = <-suite.outputChan
	suite.Equal("hello world", string(msg.Content))

	tailer = s.tailers[source.Config.Path]
	os.Rename(suite.testPath, suite.testRotatedPath)
	f, err := os.Create(suite.testPath)
	suite.Nil(err)
	s.scan()
	newTailer = s.tailers[source.Config.Path]
	suite.True(tailer != newTailer)

	_, err = f.WriteString("hello again\n")
	suite.Nil(err)
	msg = <-suite.outputChan
	suite.Equal("hello again", string(msg.Content))
}

func (suite *ScannerTestSuite) TestScannerScanWithLogRotationCopyTruncate() {
	s := suite.s
	source := suite.source

	var tailer *Tailer
	var newTailer *Tailer
	var err error
	var msg *message.Message

	tailer = s.tailers[source.Config.Path]
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
	newTailer = s.tailers[source.Config.Path]
	suite.True(tailer != newTailer)

	msg = <-suite.outputChan
	suite.Equal("third", string(msg.Content))
}

func (suite *ScannerTestSuite) TestScannerScanWithFileRemovedAndCreated() {
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

func (suite *ScannerTestSuite) TestLifeCycle() {
	s := suite.s
	suite.Equal(1, len(s.tailers))
	s.Start()

	// all tailers should be stopped
	s.Stop()
	suite.Equal(0, len(s.tailers))
}

func TestScannerTestSuite(t *testing.T) {
	suite.Run(t, new(ScannerTestSuite))
}

func TestScannerScanStartNewTailer(t *testing.T) {
	var err error
	var path string
	var file *os.File
	var tailer *Tailer
	var msg *message.Message

	testDir, err := ioutil.TempDir("", "log-scanner-test-")
	assert.Nil(t, err)

	// create scanner
	path = fmt.Sprintf("%s/*.log", testDir)
	openFilesLimit := 2
	sleepDuration := 20 * time.Millisecond
	scanner := NewScanner(config.NewLogSources(), openFilesLimit, mock.NewMockProvider(), auditor.NewRegistry(), sleepDuration)
	source := config.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: path})
	scanner.activeSources = append(scanner.activeSources, source)
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
	scanner.scan()
	assert.Equal(t, 1, len(scanner.tailers))
	tailer = scanner.tailers[path]
	msg = <-tailer.outputChan
	assert.Equal(t, "hello", string(msg.Content))
	msg = <-tailer.outputChan
	assert.Equal(t, "world", string(msg.Content))
}

func TestScannerTailFromTheBeginning(t *testing.T) {
	var err error
	var path string
	var file *os.File
	var tailer *Tailer
	var msg *message.Message

	testDir, err := ioutil.TempDir("", "log-scanner-test-")
	path = fmt.Sprintf("%s/test.log", testDir)
	assert.Nil(t, err)

	// create scanner
	openFilesLimit := 2
	sleepDuration := 20 * time.Millisecond
	scanner := NewScanner(config.NewLogSources(), openFilesLimit, mock.NewMockProvider(), auditor.NewRegistry(), sleepDuration)
	source := config.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: path, TailingMode: "beginning"})
	// scanner.activeSources = append(scanner.activeSources, source)
	status.Clear()
	status.InitStatus(config.CreateSources([]*config.LogSource{source}))
	defer status.Clear()

	// create file
	file, err = os.Create(path)
	assert.Nil(t, err)

	// add content before starting the tailer
	_, err = file.WriteString("Once\n")
	assert.Nil(t, err)
	_, err = file.WriteString("Upon\n")
	assert.Nil(t, err)

	// test scan from the beginning, it shall read previously written strings
	scanner.addSource(source)
	assert.Equal(t, 1, len(scanner.tailers))

	// add content after starting the tailer
	_, err = file.WriteString("A\n")
	assert.Nil(t, err)
	_, err = file.WriteString("Time\n")
	assert.Nil(t, err)

	tailer = scanner.tailers[path]
	msg = <-tailer.outputChan
	assert.Equal(t, "Once", string(msg.Content))
	msg = <-tailer.outputChan
	assert.Equal(t, "Upon", string(msg.Content))
	msg = <-tailer.outputChan
	assert.Equal(t, "A", string(msg.Content))
	msg = <-tailer.outputChan
	assert.Equal(t, "Time", string(msg.Content))
}

func TestScannerScanWithTooManyFiles(t *testing.T) {
	var err error
	var path string

	testDir, err := ioutil.TempDir("", "log-scanner-test-")
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

	// create scanner
	path = fmt.Sprintf("%s/*.log", testDir)
	openFilesLimit := 2
	sleepDuration := 20 * time.Millisecond
	scanner := NewScanner(config.NewLogSources(), openFilesLimit, mock.NewMockProvider(), auditor.NewRegistry(), sleepDuration)
	source := config.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: path})
	scanner.activeSources = append(scanner.activeSources, source)
	status.Clear()
	status.InitStatus(config.CreateSources([]*config.LogSource{source}))
	defer status.Clear()

	// test at scan
	scanner.scan()
	assert.Equal(t, 2, len(scanner.tailers))

	path = fmt.Sprintf("%s/2.log", testDir)
	err = os.Remove(path)
	assert.Nil(t, err)

	scanner.scan()
	assert.Equal(t, 1, len(scanner.tailers))

	scanner.scan()
	assert.Equal(t, 2, len(scanner.tailers))
}
