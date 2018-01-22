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
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/mock"
)

var chanSize = 10

type TailerTestSuite struct {
	suite.Suite
	testDir  string
	testPath string
	testFile *os.File

	tl         *Tailer
	outputChan chan message.Message
	source     *config.IntegrationConfigLogSource
}

func (suite *TailerTestSuite) SetupTest() {
	var err error
	suite.testDir, err = ioutil.TempDir("", "log-tailer-test-")
	suite.Nil(err)

	suite.testPath = fmt.Sprintf("%s/tailer.log", suite.testDir)
	f, err := os.Create(suite.testPath)
	suite.Nil(err)
	suite.testFile = f
	suite.outputChan = make(chan message.Message, chanSize)
	suite.source = &config.IntegrationConfigLogSource{
		Type:    config.FileType,
		Path:    suite.testPath,
		Tracker: status.NewTracker(),
	}
	suite.tl = NewTailer(suite.outputChan, suite.source, suite.testPath)
	suite.tl.sleepDuration = 10 * time.Millisecond
}

func (suite *TailerTestSuite) TearDownTest() {
	suite.tl.Stop(false)
	suite.testFile.Close()
	os.Remove(suite.testDir)
}

func (suite *TailerTestSuite) TestTailFromBeginning() {
	lines := []string{"hello world\n", "hello again\n", "good bye\n"}

	var msg message.Message
	var err error

	// this line should be tailed
	_, err = suite.testFile.WriteString(lines[0])
	suite.Nil(err)

	suite.tl.tailFromBeginning()

	// those lines should be tailed
	_, err = suite.testFile.WriteString(lines[1])
	suite.Nil(err)
	_, err = suite.testFile.WriteString(lines[2])
	suite.Nil(err)

	msg = <-suite.outputChan
	suite.Equal("hello world", string(msg.Content()))
	suite.Equal(len(lines[0]), int(msg.GetOrigin().Offset))

	msg = <-suite.outputChan
	suite.Equal("hello again", string(msg.Content()))
	suite.Equal(len(lines[0])+len(lines[1]), int(msg.GetOrigin().Offset))

	msg = <-suite.outputChan
	suite.Equal("good bye", string(msg.Content()))
	suite.Equal(len(lines[0])+len(lines[1])+len(lines[2]), int(msg.GetOrigin().Offset))

	suite.Equal(len(lines[0])+len(lines[1])+len(lines[2]), int(suite.tl.GetReadOffset()))
}

func (suite *TailerTestSuite) TestRecoverTailing() {
	lines := []string{"hello world\n", "hello again\n", "good bye\n"}

	var msg message.Message
	var err error

	// those line should be skipped
	_, err = suite.testFile.WriteString(lines[0])
	suite.Nil(err)

	// this line should be tailed
	_, err = suite.testFile.WriteString(lines[1])
	suite.Nil(err)

	suite.tl.recoverTailing(int64(len(lines[0])), os.SEEK_CUR)

	// this line should be tailed
	_, err = suite.testFile.WriteString(lines[2])
	suite.Nil(err)

	msg = <-suite.outputChan
	suite.Equal("hello again", string(msg.Content()))
	suite.Equal(len(lines[0])+len(lines[1]), int(msg.GetOrigin().Offset))

	msg = <-suite.outputChan
	suite.Equal("good bye", string(msg.Content()))
	suite.Equal(len(lines[0])+len(lines[1])+len(lines[2]), int(msg.GetOrigin().Offset))

	suite.Equal(len(lines[0])+len(lines[1])+len(lines[2]), int(suite.tl.GetReadOffset()))
}

func (suite *TailerTestSuite) TestTailerIdentifier() {
	suite.Equal(fmt.Sprintf("file:%s/tailer.log", suite.testDir), suite.tl.Identifier())
}

func writeMessage(file *os.File) {
	file.WriteString("hello world\n")
}

func listenToChan(inputChan chan *decoder.Input, messagesReceived *uint64) {
	for range inputChan {
		atomic.AddUint64(messagesReceived, 1)
		tick()
	}
}

func tick() {
	time.Sleep(10 * time.Millisecond)
}

func TestTailerTestSuite(t *testing.T) {
	suite.Run(t, new(TailerTestSuite))
}
