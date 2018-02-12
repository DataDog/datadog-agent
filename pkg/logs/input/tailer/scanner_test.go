// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package tailer

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline/mock"
)

type ScannerTestSuite struct {
	suite.Suite
	testDir         string
	testPath        string
	testFile        *os.File
	testRotatedPath string
	testRotatedFile *os.File

	outputChan     chan message.Message
	pp             pipeline.Provider
	sources        []*config.LogSource
	openFilesLimit int
	s              *Scanner
}

func (suite *ScannerTestSuite) SetupTest() {
	suite.pp = mock.NewMockProvider()
	suite.outputChan = suite.pp.NextPipelineChan()

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
	suite.sources = []*config.LogSource{config.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: suite.testPath})}
	sleepDuration := 20 * time.Millisecond
	suite.s = New(suite.sources, suite.openFilesLimit, suite.pp, auditor.New(nil, ""), sleepDuration)
	suite.s.setup()
}

func (suite *ScannerTestSuite) TearDownTest() {
	suite.testFile.Close()
	suite.testRotatedFile.Close()
	os.Remove(suite.testDir)
}

func (suite *ScannerTestSuite) TestScannerStartsTailers() {
	_, err := suite.testFile.WriteString("hello world\n")
	suite.Nil(err)
	msg := <-suite.outputChan
	suite.Equal("hello world", string(msg.Content()))
}

func (suite *ScannerTestSuite) TestScannerScanWithoutLogRotation() {
	s := suite.s
	sources := suite.sources

	var tailer *Tailer
	var newTailer *Tailer
	var err error
	var msg message.Message

	tailer = s.tailers[sources[0].Config.Path]
	_, err = suite.testFile.WriteString("hello world\n")
	suite.Nil(err)
	msg = <-suite.outputChan
	suite.Equal("hello world", string(msg.Content()))

	s.scan()
	newTailer = s.tailers[sources[0].Config.Path]
	// testing that scanner did not have to create a new tailer
	suite.True(tailer == newTailer)

	_, err = suite.testFile.WriteString("hello again\n")
	suite.Nil(err)
	msg = <-suite.outputChan
	suite.Equal("hello again", string(msg.Content()))
}

func (suite *ScannerTestSuite) TestScannerScanWithLogRotation() {
	s := suite.s
	sources := suite.sources

	var tailer *Tailer
	var newTailer *Tailer
	var err error
	var msg message.Message

	_, err = suite.testFile.WriteString("hello world\n")
	suite.Nil(err)
	msg = <-suite.outputChan
	suite.Equal("hello world", string(msg.Content()))

	tailer = s.tailers[sources[0].Config.Path]
	os.Rename(suite.testPath, suite.testRotatedPath)
	f, err := os.Create(suite.testPath)
	suite.Nil(err)
	s.scan()
	newTailer = s.tailers[sources[0].Config.Path]
	suite.True(tailer != newTailer)

	_, err = f.WriteString("hello again\n")
	suite.Nil(err)
	msg = <-suite.outputChan
	suite.Equal("hello again", string(msg.Content()))
}

func (suite *ScannerTestSuite) TestScannerScanWithLogRotationCopyTruncate() {
	s := suite.s
	sources := suite.sources

	var tailer *Tailer
	var newTailer *Tailer
	var err error
	var msg message.Message

	tailer = s.tailers[sources[0].Config.Path]
	_, err = suite.testFile.WriteString("hello world\n")
	suite.Nil(err)
	msg = <-suite.outputChan
	suite.Equal("hello world", string(msg.Content()))

	suite.testFile.Truncate(0)
	suite.testFile.Seek(0, 0)
	f, err := os.OpenFile(suite.testPath, os.O_APPEND|os.O_WRONLY, os.ModeAppend)
	suite.Nil(err)
	_, err = f.WriteString("third\n")
	suite.Nil(err)
	s.scan()
	newTailer = s.tailers[sources[0].Config.Path]
	suite.True(tailer != newTailer)

	msg = <-suite.outputChan
	suite.Equal("third", string(msg.Content()))
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
	sources := []*config.LogSource{config.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: path})}
	openFilesLimit := 2
	sleepDuration := 20 * time.Millisecond
	scanner := New(sources, openFilesLimit, mock.NewMockProvider(), auditor.New(nil, ""), sleepDuration)

	// test at setup
	scanner.setup()
	assert.Equal(t, 2, len(scanner.tailers))

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
