// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

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

type TailerTestSuite struct {
	suite.Suite
	testDir  string
	testPath string
	testFile *os.File

	tl         *Tailer
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
	suite.tl = NewTailer(suite.outputChan, suite.source, suite.testPath, sleepDuration)
}

func (suite *TailerTestSuite) TearDownTest() {
	suite.tl.Stop()
	suite.testFile.Close()
	os.Remove(suite.testDir)
}

func (suite *TailerTestSuite) TestTailFromBeginning() {
	lines := []string{"hello world\n", "hello again\n", "good bye\n"}

	var msg *message.Message
	var err error

	// this line should be tailed
	_, err = suite.testFile.WriteString(lines[0])
	suite.Nil(err)

	suite.tl.StartFromBeginning()

	// those lines should be tailed
	_, err = suite.testFile.WriteString(lines[1])
	suite.Nil(err)
	_, err = suite.testFile.WriteString(lines[2])
	suite.Nil(err)

	msg = <-suite.outputChan
	suite.Equal("hello world", string(msg.Content))
	suite.Equal(len(lines[0]), toInt(msg.GetOrigin().Offset))

	msg = <-suite.outputChan
	suite.Equal("hello again", string(msg.Content))
	suite.Equal(len(lines[0])+len(lines[1]), toInt(msg.GetOrigin().Offset))

	msg = <-suite.outputChan
	suite.Equal("good bye", string(msg.Content))
	suite.Equal(len(lines[0])+len(lines[1])+len(lines[2]), toInt(msg.GetOrigin().Offset))

	suite.Equal(len(lines[0])+len(lines[1])+len(lines[2]), int(suite.tl.GetDecodedOffset()))
}

func (suite *TailerTestSuite) TestTailFromEnd() {
	lines := []string{"hello world\n", "hello again\n", "good bye\n"}

	var msg *message.Message
	var err error

	// this line should be tailed
	_, err = suite.testFile.WriteString(lines[0])
	suite.Nil(err)

	suite.tl.Start(0, io.SeekEnd)

	// those lines should be tailed
	_, err = suite.testFile.WriteString(lines[1])
	suite.Nil(err)
	_, err = suite.testFile.WriteString(lines[2])
	suite.Nil(err)

	msg = <-suite.outputChan
	suite.Equal("hello again", string(msg.Content))
	suite.Equal(len(lines[0])+len(lines[1]), toInt(msg.GetOrigin().Offset))

	msg = <-suite.outputChan
	suite.Equal("good bye", string(msg.Content))
	suite.Equal(len(lines[0])+len(lines[1])+len(lines[2]), toInt(msg.GetOrigin().Offset))

	suite.Equal(len(lines[0])+len(lines[1])+len(lines[2]), int(suite.tl.GetDecodedOffset()))
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

	suite.tl.Start(int64(len(lines[0])), io.SeekStart)

	// this line should be tailed
	_, err = suite.testFile.WriteString(lines[2])
	suite.Nil(err)

	msg = <-suite.outputChan
	suite.Equal("hello again", string(msg.Content))
	suite.Equal(len(lines[0])+len(lines[1]), toInt(msg.GetOrigin().Offset))

	msg = <-suite.outputChan
	suite.Equal("good bye", string(msg.Content))
	suite.Equal(len(lines[0])+len(lines[1])+len(lines[2]), toInt(msg.GetOrigin().Offset))

	suite.Equal(len(lines[0])+len(lines[1])+len(lines[2]), int(suite.tl.GetDecodedOffset()))
}

func (suite *TailerTestSuite) TestTailerIdentifier() {
	suite.tl.StartFromBeginning()
	suite.Equal(fmt.Sprintf("file:%s/tailer.log", suite.testDir), suite.tl.Identifier())
}

func (suite *TailerTestSuite) TestOriginTagsWhenTailingFiles() {

	suite.tl.StartFromBeginning()

	_, err := suite.testFile.WriteString("foo\n")
	suite.Nil(err)

	msg := <-suite.outputChan
	tags := msg.GetOrigin().Tags()
	suite.Equal(1, len(tags))
	suite.Equal("filename:"+filepath.Base(suite.testFile.Name()), tags[0])

}

func TestTailerTestSuite(t *testing.T) {
	suite.Run(t, new(TailerTestSuite))
}

func toInt(str string) int {
	if value, err := strconv.ParseInt(str, 10, 64); err == nil {
		return int(value)
	}
	return 0
}
