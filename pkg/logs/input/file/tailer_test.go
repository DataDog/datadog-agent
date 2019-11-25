// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !windows

package file

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

var chanSize = 10
var closeTimeout = 1 * time.Second

type TailerTestSuite struct {
	suite.Suite
	testDir  string
	testPath string
	testFile *os.File

	tailer     *Tailer
	outputChan chan *message.Message
	source     *config.LogSource
}

func (suite *TailerTestSuite) SetupTest() {
	var err error
	suite.testDir, err = ioutil.TempDir("", "log-tailer-test-")
	suite.Nil(err)

	suite.testPath = fmt.Sprintf("%s/tailer.log", suite.testDir)
	f, err := os.Create(suite.testPath)
	suite.Nil(err)
	suite.testFile = f
	suite.outputChan = make(chan *message.Message, chanSize)
	suite.source = config.NewLogSource("", &config.LogsConfig{
		Type: config.FileType,
		Path: suite.testPath,
	})
	sleepDuration := 10 * time.Millisecond
	suite.tailer = NewTailer(suite.outputChan, suite.source, suite.testPath, sleepDuration, false)
	suite.tailer.closeTimeout = closeTimeout
}

func (suite *TailerTestSuite) TearDownTest() {
	suite.tailer.Stop()
	suite.testFile.Close()
	os.Remove(suite.testDir)
}

func TestTailerTestSuite(t *testing.T) {
	suite.Run(t, new(TailerTestSuite))
}

func (suite *TailerTestSuite) TestStopAfterFileRotationWhenStuck() {
	// Write more messages than the output channel capacity
	for i := 0; i < chanSize+2; i++ {
		_, err := suite.testFile.WriteString(fmt.Sprintf("line %d\n", i))
		suite.Nil(err)
	}

	// Start to tail and ensure it has read the file
	// At this point the tailer is stuck because the channel is full
	// and it tries to write in it
	err := suite.tailer.StartFromBeginning()
	suite.Nil(err)
	<-suite.tailer.outputChan

	// Ask the tailer to stop after a file rotation
	suite.tailer.StopAfterFileRotation()

	// Ensure the tailer is effectively closed
	select {
	case <-suite.tailer.done:
	case <-time.After(closeTimeout + 10*time.Second):
		suite.Fail("timeout")
	}
}

func (suite *TailerTestSuite) TestTailFromBeginning() {
	lines := []string{"hello world\n", "hello again\n", "good bye\n"}

	var msg *message.Message
	var err error

	// this line should be tailed
	_, err = suite.testFile.WriteString(lines[0])
	suite.Nil(err)

	suite.tailer.StartFromBeginning()

	// those lines should be tailed
	_, err = suite.testFile.WriteString(lines[1])
	suite.Nil(err)
	_, err = suite.testFile.WriteString(lines[2])
	suite.Nil(err)

	msg = <-suite.outputChan
	suite.Equal("hello world", string(msg.Content))
	suite.Equal(len(lines[0]), toInt(msg.Origin.Offset))

	msg = <-suite.outputChan
	suite.Equal("hello again", string(msg.Content))
	suite.Equal(len(lines[0])+len(lines[1]), toInt(msg.Origin.Offset))

	msg = <-suite.outputChan
	suite.Equal("good bye", string(msg.Content))
	suite.Equal(len(lines[0])+len(lines[1])+len(lines[2]), toInt(msg.Origin.Offset))

	suite.Equal(len(lines[0])+len(lines[1])+len(lines[2]), int(suite.tailer.decodedOffset))
}

func (suite *TailerTestSuite) TestTailFromEnd() {
	lines := []string{"hello world\n", "hello again\n", "good bye\n"}

	var msg *message.Message
	var err error

	// this line should be tailed
	_, err = suite.testFile.WriteString(lines[0])
	suite.Nil(err)

	suite.tailer.Start(0, io.SeekEnd)

	// those lines should be tailed
	_, err = suite.testFile.WriteString(lines[1])
	suite.Nil(err)
	_, err = suite.testFile.WriteString(lines[2])
	suite.Nil(err)

	msg = <-suite.outputChan
	suite.Equal("hello again", string(msg.Content))
	suite.Equal(len(lines[0])+len(lines[1]), toInt(msg.Origin.Offset))

	msg = <-suite.outputChan
	suite.Equal("good bye", string(msg.Content))
	suite.Equal(len(lines[0])+len(lines[1])+len(lines[2]), toInt(msg.Origin.Offset))

	suite.Equal(len(lines[0])+len(lines[1])+len(lines[2]), int(suite.tailer.decodedOffset))
}

func (suite *TailerTestSuite) TestRecoverTailing() {
	lines := []string{"hello world\n", "hello again\n", "good bye\n"}

	var msg *message.Message
	var err error

	// those line should be skipped
	_, err = suite.testFile.WriteString(lines[0])
	suite.Nil(err)

	// this line should be tailed
	_, err = suite.testFile.WriteString(lines[1])
	suite.Nil(err)

	suite.tailer.Start(int64(len(lines[0])), io.SeekStart)

	// this line should be tailed
	_, err = suite.testFile.WriteString(lines[2])
	suite.Nil(err)

	msg = <-suite.outputChan
	suite.Equal("hello again", string(msg.Content))
	suite.Equal(len(lines[0])+len(lines[1]), toInt(msg.Origin.Offset))

	msg = <-suite.outputChan
	suite.Equal("good bye", string(msg.Content))
	suite.Equal(len(lines[0])+len(lines[1])+len(lines[2]), toInt(msg.Origin.Offset))

	suite.Equal(len(lines[0])+len(lines[1])+len(lines[2]), int(suite.tailer.decodedOffset))
}

func (suite *TailerTestSuite) TestTailerIdentifier() {
	suite.tailer.StartFromBeginning()
	suite.Equal(fmt.Sprintf("file:%s/tailer.log", suite.testDir), suite.tailer.Identifier())
}

func (suite *TailerTestSuite) TestOriginTagsWhenTailingFiles() {

	suite.tailer.StartFromBeginning()

	_, err := suite.testFile.WriteString("foo\n")
	suite.Nil(err)

	msg := <-suite.outputChan
	tags := msg.Origin.Tags()
	suite.Equal(1, len(tags))
	suite.Equal("filename:"+filepath.Base(suite.testFile.Name()), tags[0])
}

func (suite *TailerTestSuite) TestDirTagWhenTailingFiles() {

	dirTaggedSource := config.NewLogSource("", &config.LogsConfig{
		Type: config.FileType,
		Path: suite.testPath,
	})
	sleepDuration := 10 * time.Millisecond
	suite.tailer = NewTailer(suite.outputChan, dirTaggedSource, suite.testPath, sleepDuration, true)
	suite.tailer.StartFromBeginning()

	_, err := suite.testFile.WriteString("foo\n")
	suite.Nil(err)

	msg := <-suite.outputChan
	tags := msg.Origin.Tags()
	suite.Equal(2, len(tags))
	suite.Equal("filename:"+filepath.Base(suite.testFile.Name()), tags[0])
	suite.Equal("dirname:"+filepath.Dir(suite.testFile.Name()), tags[1])
}

func (suite *TailerTestSuite) TestBuildTagsFileOnly() {
	dirTaggedSource := config.NewLogSource("", &config.LogsConfig{
		Type: config.FileType,
		Path: suite.testPath,
	})
	sleepDuration := 10 * time.Millisecond
	suite.tailer = NewTailer(suite.outputChan, dirTaggedSource, suite.testPath, sleepDuration, false)
	suite.tailer.StartFromBeginning()

	tags := suite.tailer.buildTailerTags()
	suite.Equal(1, len(tags))
	suite.Equal("filename:"+filepath.Base(suite.testFile.Name()), tags[0])
}

func (suite *TailerTestSuite) TestBuildTagsFileDir() {
	dirTaggedSource := config.NewLogSource("", &config.LogsConfig{
		Type: config.FileType,
		Path: suite.testPath,
	})
	sleepDuration := 10 * time.Millisecond
	suite.tailer = NewTailer(suite.outputChan, dirTaggedSource, suite.testPath, sleepDuration, true)
	suite.tailer.StartFromBeginning()

	tags := suite.tailer.buildTailerTags()
	suite.Equal(2, len(tags))
	suite.Equal("filename:"+filepath.Base(suite.testFile.Name()), tags[0])
	suite.Equal("dirname:"+filepath.Dir(suite.testFile.Name()), tags[1])
}

func toInt(str string) int {
	if value, err := strconv.ParseInt(str, 10, 64); err == nil {
		return int(value)
	}
	return 0
}
