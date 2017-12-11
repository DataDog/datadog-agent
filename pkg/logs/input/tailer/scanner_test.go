// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build !windows

package tailer

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/stretchr/testify/suite"
)

type ScannerTestSuite struct {
	suite.Suite
	testDir         string
	testPath        string
	testFile        *os.File
	testRotatedPath string
	testRotatedFile *os.File

	outputChan chan message.Message
	pp         *pipeline.Provider
	sources    []*config.IntegrationConfigLogSource
	s          *Scanner
}

func (suite *ScannerTestSuite) SetupTest() {
	suite.pp = pipeline.NewProvider()
	suite.pp.MockPipelineChans()
	suite.outputChan = suite.pp.NextPipelineChan()

	suite.testDir = "tests/scanner"
	os.Remove(suite.testDir)
	os.MkdirAll(suite.testDir, os.ModeDir)
	suite.testPath = fmt.Sprintf("%s/scanner.log", suite.testDir)
	suite.testRotatedPath = fmt.Sprintf("%s.1", suite.testPath)

	f, err := os.Create(suite.testPath)
	suite.Nil(err)
	suite.testFile = f
	f, err = os.Create(suite.testRotatedPath)
	suite.Nil(err)
	suite.testRotatedFile = f

	suite.sources = []*config.IntegrationConfigLogSource{{Type: config.FileType, Path: suite.testPath}}
	suite.s = New(suite.sources, suite.pp, auditor.New(nil))
	suite.s.setup()
	for _, tl := range suite.s.tailers {
		tl.sleepMutex.Lock()
		tl.sleepDuration = 100 * time.Millisecond
		tl.sleepMutex.Unlock()
	}
}

func (suite *ScannerTestSuite) TearDownTest() {
	suite.s.Stop()

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
	var readOffset int64

	tailer = s.tailers[sources[0].Path]
	_, err = suite.testFile.WriteString("hello world\n")
	suite.Nil(err)
	msg = <-suite.outputChan
	suite.Equal("hello world", string(msg.Content()))
	readOffset = tailer.GetReadOffset()
	suite.True(readOffset > 0)

	s.scan()
	newTailer = s.tailers[sources[0].Path]
	// testing that scanner did not have to create a new tailer
	suite.True(tailer == newTailer)

	_, err = suite.testFile.WriteString("hello again\n")
	suite.Nil(err)
	msg = <-suite.outputChan
	suite.Equal("hello again", string(msg.Content()))
	suite.True(tailer.GetReadOffset() > readOffset)
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

	tailer = s.tailers[sources[0].Path]
	os.Rename(suite.testPath, suite.testRotatedPath)
	f, err := os.Create(suite.testPath)
	suite.Nil(err)
	s.scan()
	newTailer = s.tailers[sources[0].Path]
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

	tailer = s.tailers[sources[0].Path]
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
	newTailer = s.tailers[sources[0].Path]
	suite.True(tailer != newTailer)

	msg = <-suite.outputChan
	suite.Equal("third", string(msg.Content()))
}

func TestScannerTestSuite(t *testing.T) {
	suite.Run(t, new(ScannerTestSuite))
}
