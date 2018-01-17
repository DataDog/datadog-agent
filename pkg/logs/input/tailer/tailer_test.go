// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package tailer

// import (
// 	"fmt"
// 	"io/ioutil"
// 	"os"
// 	"sync/atomic"
// 	"testing"
// 	"time"

// 	"github.com/stretchr/testify/suite"

// 	"github.com/DataDog/datadog-agent/pkg/logs/config"
// 	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
// 	"github.com/DataDog/datadog-agent/pkg/logs/message"
// )

// var chanSize = 10

// type TailerTestSuite struct {
// 	suite.Suite
// 	testDir  string
// 	testPath string
// 	testFile *os.File

// 	tl         *Tailer
// 	outputChan chan message.Message
// 	source     *config.IntegrationConfigLogSource
// }

// func (suite *TailerTestSuite) SetupTest() {
// 	var err error
// 	suite.testDir, err = ioutil.TempDir("", "log-tailer-test-")
// 	suite.Nil(err)

// 	suite.testPath = fmt.Sprintf("%s/tailer.log", suite.testDir)
// 	f, err := os.Create(suite.testPath)
// 	suite.Nil(err)
// 	suite.testFile = f
// 	suite.outputChan = make(chan message.Message, chanSize)
// 	suite.source = &config.IntegrationConfigLogSource{
// 		Type: config.FileType,
// 		Path: suite.testPath,
// 	}
// 	suite.tl = NewTailer(suite.outputChan, suite.source, suite.testPath)
// 	suite.tl.sleepDuration = 10 * time.Millisecond
// }

// func (suite *TailerTestSuite) TearDownTest() {
// 	suite.tl.Stop(false)
// 	suite.testFile.Close()
// 	os.Remove(suite.testDir)
// }

// func (suite *TailerTestSuite) TestTailerTails() {
// 	suite.tl.tailFromEnd()

// 	var msg message.Message
// 	var err error
// 	_, err = suite.testFile.WriteString("hello world\n")
// 	suite.Nil(err)
// 	_, err = suite.testFile.WriteString("hello again\n")
// 	suite.Nil(err)
// 	msg = <-suite.outputChan
// 	suite.Equal("hello world", string(msg.Content()))
// 	msg = <-suite.outputChan
// 	suite.Equal("hello again", string(msg.Content()))

// 	suite.Equal(fmt.Sprintf("file:%s/tailer.log", suite.testDir), suite.tl.Identifier())
// }

// func (suite *TailerTestSuite) TestTailerIdentifier() {
// 	suite.Equal(fmt.Sprintf("file:%s/tailer.log", suite.testDir), suite.tl.Identifier())
// }

// func (suite *TailerTestSuite) TestTailerLifecycle() {
// 	suite.tl.tailFromEnd()
// 	suite.tl.Stop(false)
// 	// FIXME: for now there is now way to know it properly stopped.
// 	// this will be fixed when we implement stop pills
// }

// func writeMessage(file *os.File) {
// 	time.Sleep(time.Millisecond)
// 	file.WriteString("hello world\n")
// }

// func listenToChan(inputChan chan *decoder.Input, messagesReceived *uint64) {
// 	for range inputChan {
// 		atomic.AddUint64(messagesReceived, 1)
// 		tick()
// 	}
// }

// func tick() {
// 	time.Sleep(10 * time.Millisecond)
// }

// // TestTailerIsSlowAndCatchesUp tests that when the tailer
// // is delayed and we stop it, it still processes the file
// // until EOF
// func (suite *TailerTestSuite) TestTailerIsSlowAndCatchesUp() {
// 	suite.tl.sleepDuration = time.Millisecond

// 	// mock tailer output channel
// 	suite.tl.d.InputChan = make(chan *decoder.Input, 2)
// 	suite.tl.startReading(0, os.SEEK_END)

// 	// fill output channel
// 	of1 := suite.tl.GetReadOffset()
// 	for i := 0; i < 5; i++ {
// 		writeMessage(suite.testFile)
// 	}
// 	// assert we read part of the file
// 	of2 := suite.tl.GetReadOffset()
// 	suite.True(of1 < of2)

// 	// assert reads are blocked: we write in the file but
// 	// offset is unchanged
// 	for i := 0; i < 5; i++ {
// 		writeMessage(suite.testFile)
// 	}
// 	of3 := suite.tl.GetReadOffset()
// 	suite.Equal(of2, of3)

// 	// slowly process all logs in the channel
// 	tick()
// 	var messagesReceived uint64
// 	go listenToChan(suite.tl.d.InputChan, &messagesReceived)
// 	tick()
// 	received := atomic.LoadUint64(&messagesReceived)
// 	suite.True(received > 0)

// 	// Stop tailer - it should keep processing till end of file
// 	suite.tl.Stop(false)
// 	tick()

// 	// converge - it should have read all data and stopped
// 	time.Sleep(100 * time.Millisecond)
// 	suite.True(atomic.LoadUint64(&messagesReceived) > received)
// }

// // TestTailerIsTooSlowAndClosed tests that when the tailer
// // is very delayed and we stop it, it stops if it doesn't reach
// // EOF after a configured time period
// func (suite *TailerTestSuite) TestTailerIsTooSlowAndClosed() {
// 	testPath := fmt.Sprintf("%s/tailer2.log", suite.testDir)
// 	testFile, _ := os.Create(testPath)
// 	defer testFile.Close()
// 	tl := NewTailer(nil, &config.IntegrationConfigLogSource{Type: config.FileType, Path: testPath}, testPath)
// 	tl.sleepDuration = 50 * time.Millisecond
// 	tl.closeTimeout = 2 * time.Millisecond

// 	// mock tailer output channel
// 	tl.d.InputChan = make(chan *decoder.Input, 2)
// 	tl.startReading(0, os.SEEK_END)

// 	// fill output channel
// 	for i := 0; i < 20; i++ {
// 		writeMessage(testFile)
// 	}

// 	// slowly process all logs in the channel
// 	tick()
// 	var messagesReceived uint64
// 	go listenToChan(tl.d.InputChan, &messagesReceived)
// 	tick()

// 	// Stop tailer - it should keep processing till end of file
// 	// or after closeTimeout
// 	tl.Stop(false)
// 	tick()

// 	// converge - it should have stopped processing data
// 	received := atomic.LoadUint64(&messagesReceived)
// 	tick()
// 	suite.Equal(int(atomic.LoadUint64(&messagesReceived)), int(received))
// }

// func TestTailerTestSuite(t *testing.T) {
// 	suite.Run(t, new(TailerTestSuite))
// }
