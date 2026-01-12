// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package file

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
)

// TailerIntegrationTestSuite contains integration tests for the file tailer
// that test more complex scenarios involving real disk operations and
// fully functioning dependencies.
type TailerIntegrationTestSuite struct {
	suite.Suite
	testDir    string
	testPath   string
	testFile   *os.File
	outputChan chan *message.Message
	source     *sources.ReplaceableSource
}

func TestIntegrationSuite(t *testing.T) {
	suite.Run(t, new(TailerIntegrationTestSuite))
}

func (suite *TailerIntegrationTestSuite) SetupTest() {
	suite.testDir = suite.T().TempDir()
	suite.testPath = filepath.Join(suite.testDir, "tailer-integration.log")
	suite.outputChan = make(chan *message.Message, 10)
}

func (suite *TailerIntegrationTestSuite) TearDownTest() {
	if suite.testFile != nil {
		suite.testFile.Close()
	}
}

// createTailerWithEncoding creates a tailer configured for a specific encoding
func (suite *TailerIntegrationTestSuite) createTailerWithEncoding(encoding string) *Tailer {
	suite.source = sources.NewReplaceableSource(sources.NewLogSource("", &config.LogsConfig{
		Type:     config.FileType,
		Path:     suite.testPath,
		Encoding: encoding,
	}))

	sleepDuration := 10 * time.Millisecond
	info := status.NewInfoRegistry()

	options := &TailerOptions{
		OutputChan:      suite.outputChan,
		File:            NewFile(suite.testPath, suite.source.UnderlyingSource(), false),
		SleepDuration:   sleepDuration,
		Decoder:         decoder.NewDecoderFromSource(suite.source, info),
		Info:            info,
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
		Registry:        auditor.NewMockRegistry(),
		FileOpener:      opener.NewFileOpener(),
	}

	tailer := NewTailer(options)
	tailer.closeTimeout = 1 * time.Second
	return tailer
}

type encodingWriter func(f *os.File, text string) error

func (suite *TailerIntegrationTestSuite) writeUTF16LE(f *os.File, text string) error {
	utf16Codes := utf16.Encode([]rune(text))

	if err := binary.Write(f, binary.LittleEndian, utf16Codes); err != nil {
		return err
	}
	return nil
}

func (suite *TailerIntegrationTestSuite) writeUTF16BE(f *os.File, text string) error {
	utf16Codes := utf16.Encode([]rune(text))

	if err := binary.Write(f, binary.BigEndian, utf16Codes); err != nil {
		return err
	}
	return nil
}

func (suite *TailerIntegrationTestSuite) writeUTF8(f *os.File, text string) error {
	_, err := f.WriteString(text)
	return err
}

// TestTailerUTF16LE tests that the tailer correctly decodes UTF-16 Little Endian encoded log files
func (suite *TailerIntegrationTestSuite) TestTailerEncodings() {
	suite.encodingTestRunner(config.UTF16LE, suite.writeUTF16LE)
	suite.encodingTestRunner(config.UTF16BE, suite.writeUTF16BE)
	suite.encodingTestRunner("random value/default", suite.writeUTF8)
}

// TestTailerUTF16BE tests that the tailer correctly decodes UTF-16 Big Endian encoded log files
func (suite *TailerIntegrationTestSuite) encodingTestRunner(encoding string, encoder encodingWriter) {
	var err error

	suite.testFile, err = os.Create(suite.testPath)
	suite.Require().NoError(err)
	suite.Require().NotNil(suite.testFile)
	defer suite.testFile.Close()

	tailer := suite.createTailerWithEncoding(encoding)
	defer tailer.Stop()

	testLine1 := "sample log: 🎉 <- this is an emoji"
	err = encoder(suite.testFile, testLine1+"\n")
	suite.Require().NoError(err)
	suite.testFile.Sync()

	err = tailer.StartFromBeginning()
	suite.Require().NoError(err)

	testLine2 := "second line: 日本語 <- this is a Japanese character"
	err = encoder(suite.testFile, testLine2+"\n")
	suite.Require().NoError(err)
	suite.testFile.Sync()

	select {
	case msg := <-suite.outputChan:
		suite.Equal(testLine1, string(msg.GetContent()), "expectected identical lines for encoding: "+encoding)
	case <-time.After(1 * time.Second):
		suite.Fail("timeout waiting for first message, there was likely an error decoding the newline", "encoding: "+encoding)
	}

	select {
	case msg := <-suite.outputChan:
		suite.Equal(testLine2, string(msg.GetContent()), "expectected identical lines for encoding: "+encoding)
	case <-time.After(1 * time.Second):
		suite.Fail("timeout waiting for second message, there was likely an error decoding the newline", "encoding: "+encoding)
	}
}
