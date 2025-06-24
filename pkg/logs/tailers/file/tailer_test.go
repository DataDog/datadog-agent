// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"fmt"
	"hash/crc64"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
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
	source     *sources.ReplaceableSource
}

func (suite *TailerTestSuite) SetupTest() {
	var err error
	suite.testDir = suite.T().TempDir()

	suite.testPath = filepath.Join(suite.testDir, "tailer.log")
	f, err := os.Create(suite.testPath)
	suite.NotNil(f)
	suite.Nil(err)
	suite.testFile = f
	suite.outputChan = make(chan *message.Message, chanSize)
	suite.source = sources.NewReplaceableSource(sources.NewLogSource("", &config.LogsConfig{
		Type: config.FileType,
		Path: suite.testPath,
	}))
	sleepDuration := 10 * time.Millisecond
	info := status.NewInfoRegistry()

	tailerOptions := &TailerOptions{
		OutputChan:      suite.outputChan,
		File:            NewFile(suite.testPath, suite.source.UnderlyingSource(), false),
		SleepDuration:   sleepDuration,
		Decoder:         decoder.NewDecoderFromSource(suite.source, info),
		Info:            info,
		PipelineMonitor: metrics.NewNoopPipelineMonitor(""),
	}

	suite.tailer = NewTailer(tailerOptions)
	suite.tailer.closeTimeout = closeTimeout
}

func (suite *TailerTestSuite) TearDownTest() {
	suite.tailer.Stop()
	suite.testFile.Close()
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

func (suite *TailerTestSuite) TestTailerTimeDurationConfig() {
	mockConfig := configmock.New(suite.T())
	// To satisfy the suite level tailer
	suite.tailer.StartFromBeginning()

	mockConfig.SetWithoutSource("logs_config.close_timeout", 42)
	sleepDuration := 10 * time.Millisecond
	info := status.NewInfoRegistry()

	tailerOptions := &TailerOptions{
		OutputChan:      suite.outputChan,
		File:            NewFile(suite.testPath, suite.source.UnderlyingSource(), false),
		SleepDuration:   sleepDuration,
		Decoder:         decoder.NewDecoderFromSource(suite.source, info),
		Info:            info,
		PipelineMonitor: metrics.NewNoopPipelineMonitor(""),
	}

	tailer := NewTailer(tailerOptions)
	tailer.StartFromBeginning()

	suite.Equal(tailer.closeTimeout, time.Duration(42)*time.Second)
	tailer.Stop()
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
	suite.Equal("hello world", string(msg.GetContent()))
	suite.Equal(len(lines[0]), toInt(msg.Origin.Offset))

	msg = <-suite.outputChan
	suite.Equal("hello again", string(msg.GetContent()))
	suite.Equal(len(lines[0])+len(lines[1]), toInt(msg.Origin.Offset))

	msg = <-suite.outputChan
	suite.Equal("good bye", string(msg.GetContent()))
	suite.Equal(len(lines[0])+len(lines[1])+len(lines[2]), toInt(msg.Origin.Offset))

	suite.Equal(len(lines[0])+len(lines[1])+len(lines[2]), int(suite.tailer.decodedOffset.Load()))
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
	suite.Equal("hello again", string(msg.GetContent()))
	suite.Equal(len(lines[0])+len(lines[1]), toInt(msg.Origin.Offset))

	msg = <-suite.outputChan
	suite.Equal("good bye", string(msg.GetContent()))
	suite.Equal(len(lines[0])+len(lines[1])+len(lines[2]), toInt(msg.Origin.Offset))

	suite.Equal(len(lines[0])+len(lines[1])+len(lines[2]), int(suite.tailer.decodedOffset.Load()))
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
	suite.Equal("hello again", string(msg.GetContent()))
	suite.Equal(len(lines[0])+len(lines[1]), toInt(msg.Origin.Offset))

	msg = <-suite.outputChan
	suite.Equal("good bye", string(msg.GetContent()))
	suite.Equal(len(lines[0])+len(lines[1])+len(lines[2]), toInt(msg.Origin.Offset))

	suite.Equal(len(lines[0])+len(lines[1])+len(lines[2]), int(suite.tailer.decodedOffset.Load()))
}

func (suite *TailerTestSuite) TestWithBlanklines() {
	lines := "\t\t\t     \t\t\n    \n\n   \n\n\r\n\r\n\r\n"
	lines += "message 1\n"
	lines += "\n\n\n\n\n\n\n\n\n\t\n"
	lines += "message 2\n"
	lines += "\n\t\r\n"
	lines += "message 3\n"

	var msg *message.Message
	var err error

	_, err = suite.testFile.WriteString(lines)
	suite.Nil(err)

	suite.tailer.Start(0, io.SeekStart)

	msg = <-suite.outputChan
	suite.Equal("message 1", string(msg.GetContent()))

	msg = <-suite.outputChan
	suite.Equal("message 2", string(msg.GetContent()))

	msg = <-suite.outputChan
	suite.Equal("message 3", string(msg.GetContent()))

	suite.Equal(len(lines), int(suite.tailer.decodedOffset.Load()))
}

func (suite *TailerTestSuite) TestTailerIdentifier() {
	suite.tailer.StartFromBeginning()
	suite.Equal(
		fmt.Sprintf("file:%s", filepath.Join(suite.testDir, "tailer.log")),
		suite.tailer.Identifier())
}

func (suite *TailerTestSuite) TestOriginTagsWhenTailingFiles() {

	suite.tailer.StartFromBeginning()

	_, err := suite.testFile.WriteString("foo\n")
	suite.Nil(err)

	msg := <-suite.outputChan
	tags := msg.Tags()
	suite.ElementsMatch([]string{
		"filename:" + filepath.Base(suite.testFile.Name()),
		"dirname:" + filepath.Dir(suite.testFile.Name()),
	}, tags)
}

func (suite *TailerTestSuite) TestDirTagWhenTailingFiles() {

	dirTaggedSource := sources.NewLogSource("", &config.LogsConfig{
		Type: config.FileType,
		Path: suite.testPath,
	})
	sleepDuration := 10 * time.Millisecond
	info := status.NewInfoRegistry()

	tailerOptions := &TailerOptions{
		OutputChan:      suite.outputChan,
		File:            NewFile(suite.testPath, dirTaggedSource, true),
		SleepDuration:   sleepDuration,
		Decoder:         decoder.NewDecoderFromSource(suite.source, info),
		Info:            info,
		PipelineMonitor: metrics.NewNoopPipelineMonitor(""),
	}

	suite.tailer = NewTailer(tailerOptions)
	suite.tailer.StartFromBeginning()

	_, err := suite.testFile.WriteString("foo\n")
	suite.Nil(err)

	msg := <-suite.outputChan
	tags := msg.Tags()
	suite.ElementsMatch([]string{
		"filename:" + filepath.Base(suite.testFile.Name()),
		"dirname:" + filepath.Dir(suite.testFile.Name()),
	}, tags)
}

func (suite *TailerTestSuite) TestBuildTagsFileOnly() {
	dirTaggedSource := sources.NewLogSource("", &config.LogsConfig{
		Type: config.FileType,
		Path: suite.testPath,
	})
	sleepDuration := 10 * time.Millisecond
	info := status.NewInfoRegistry()

	tailerOptions := &TailerOptions{
		OutputChan:      suite.outputChan,
		File:            NewFile(suite.testPath, dirTaggedSource, false),
		SleepDuration:   sleepDuration,
		Decoder:         decoder.NewDecoderFromSource(suite.source, info),
		Info:            info,
		PipelineMonitor: metrics.NewNoopPipelineMonitor(""),
	}

	suite.tailer = NewTailer(tailerOptions)

	suite.tailer.StartFromBeginning()

	tags := suite.tailer.buildTailerTags()
	suite.ElementsMatch([]string{
		"filename:" + filepath.Base(suite.testFile.Name()),
		"dirname:" + filepath.Dir(suite.testFile.Name()),
	}, tags)
}

func (suite *TailerTestSuite) TestBuildTagsFileDir() {
	dirTaggedSource := sources.NewLogSource("", &config.LogsConfig{
		Type: config.FileType,
		Path: suite.testPath,
	})
	sleepDuration := 10 * time.Millisecond
	info := status.NewInfoRegistry()

	tailerOptions := &TailerOptions{
		OutputChan:      suite.outputChan,
		File:            NewFile(suite.testPath, dirTaggedSource, true),
		SleepDuration:   sleepDuration,
		Decoder:         decoder.NewDecoderFromSource(suite.source, info),
		Info:            info,
		PipelineMonitor: metrics.NewNoopPipelineMonitor(""),
	}

	suite.tailer = NewTailer(tailerOptions)
	suite.tailer.StartFromBeginning()

	tags := suite.tailer.buildTailerTags()
	suite.ElementsMatch([]string{
		"filename:" + filepath.Base(suite.testFile.Name()),
		"dirname:" + filepath.Dir(suite.testFile.Name()),
	}, tags)
}

func (suite *TailerTestSuite) TestTruncatedTag() {
	mockConfig := configmock.New(suite.T())
	mockConfig.SetWithoutSource("logs_config.max_message_size_bytes", 3)
	mockConfig.SetWithoutSource("logs_config.tag_truncated_logs", true)
	defer mockConfig.SetWithoutSource("logs_config.max_message_size_bytes", pkgconfigsetup.DefaultMaxMessageSizeBytes)
	defer mockConfig.SetWithoutSource("logs_config.tag_truncated_logs", false)

	source := sources.NewLogSource("", &config.LogsConfig{
		Type: config.FileType,
		Path: suite.testPath,
	})
	sleepDuration := 10 * time.Millisecond
	info := status.NewInfoRegistry()

	tailerOptions := &TailerOptions{
		OutputChan:      suite.outputChan,
		File:            NewFile(suite.testPath, source, true),
		SleepDuration:   sleepDuration,
		Decoder:         decoder.NewDecoderFromSource(suite.source, info),
		Info:            info,
		PipelineMonitor: metrics.NewNoopPipelineMonitor(""),
	}

	suite.tailer = NewTailer(tailerOptions)
	suite.tailer.StartFromBeginning()

	_, err := suite.testFile.WriteString("1234\n")
	suite.Nil(err)

	msg := <-suite.outputChan
	tags := msg.Tags()
	suite.Contains(tags, message.TruncatedReasonTag("single_line"))
}

func (suite *TailerTestSuite) TestMutliLineAutoDetect() {
	lines := "Jul 12, 2021 12:55:15 PM test message 1\n"
	lines += "Jul 12, 2021 12:55:15 PM test message 2\n"

	var err error

	aml := true
	suite.source.Config().AutoMultiLine = &aml
	suite.source.Config().AutoMultiLineSampleSize = 3

	sleepDuration := 10 * time.Millisecond
	info := status.NewInfoRegistry()

	tailerOptions := &TailerOptions{
		OutputChan:      suite.outputChan,
		File:            NewFile(suite.testPath, suite.source.UnderlyingSource(), true),
		SleepDuration:   sleepDuration,
		Decoder:         decoder.NewDecoderFromSource(suite.source, info),
		Info:            info,
		PipelineMonitor: metrics.NewNoopPipelineMonitor(""),
	}

	suite.tailer = NewTailer(tailerOptions)

	_, err = suite.testFile.WriteString(lines)
	suite.Nil(err)

	suite.tailer.Start(0, io.SeekStart)
	<-suite.outputChan
	<-suite.outputChan

	suite.Nil(suite.tailer.GetDetectedPattern())
	_, err = suite.testFile.WriteString(lines)
	suite.Nil(err)

	<-suite.outputChan
	<-suite.outputChan

	expectedRegex := regexp.MustCompile(`^[A-Za-z_]+ \d+, \d+ \d+:\d+:\d+ (AM|PM)`)
	suite.Equal(suite.tailer.GetDetectedPattern(), expectedRegex)
}

// Unit test to see if agent would panic when tailer's file path is empty.
func (suite *TailerTestSuite) TestDidRotateNilFullpath() {
	suite.tailer.StartFromBeginning()

	sleepDuration := 10 * time.Millisecond
	info := status.NewInfoRegistry()

	tailerOptions := &TailerOptions{
		OutputChan:      suite.outputChan,
		File:            NewFile(suite.testPath, suite.source.UnderlyingSource(), false),
		SleepDuration:   sleepDuration,
		Decoder:         decoder.NewDecoderFromSource(suite.source, info),
		Info:            info,
		PipelineMonitor: metrics.NewNoopPipelineMonitor(""),
	}

	tailer := NewTailer(tailerOptions)
	tailer.fullpath = ""
	tailer.StartFromBeginning()

	suite.NotPanics(func() {
		_, err := suite.tailer.DidRotate()
		suite.Nil(err)
	}, "Agent should not have panicked due to empty file path")
}

func toInt(str string) int {
	if value, err := strconv.ParseInt(str, 10, 64); err == nil {
		return int(value)
	}
	return 0
}

// FingerprintTestSuite tests the fingerprinting functionality
type FingerprintTestSuite struct {
	suite.Suite
	testDir  string
	testPath string
	testFile *os.File
}

func (suite *FingerprintTestSuite) SetupTest() {
	var err error
	suite.testDir = suite.T().TempDir()
	suite.testPath = filepath.Join(suite.testDir, "fingerprint_test.log")
	f, err := os.Create(suite.testPath)
	suite.NotNil(f)
	suite.Nil(err)
	suite.testFile = f
}

func (suite *FingerprintTestSuite) TearDownTest() {
	suite.testFile.Close()
}

func TestFingerprintTestSuite(t *testing.T) {
	suite.Run(t, new(FingerprintTestSuite))
}

func (suite *FingerprintTestSuite) TestFingerprintOffsetCorrection() {
	// 1. Write known content to the file
	content := "line1\nline2\nline3\nline4\nline5\n"
	_, err := suite.testFile.WriteString(content)
	suite.Require().Nil(err)

	// 2. Create a tailer and set it up
	tailer := suite.createTailerWithConfig(returnFingerprintConfig())
	initialOffset := int64(len("line1\nline2\n"))
	err = tailer.setup(initialOffset, io.SeekStart)
	suite.Require().Nil(err)

	// 3. Compute the fingerprint
	tailer.ComputeFingerPrint()

	// 4. Verify the offset is restored
	currentOffset, err := tailer.osFile.Seek(0, io.SeekCurrent)
	suite.Require().Nil(err)
	suite.Equal(initialOffset, currentOffset, "The file offset should be restored to its initial position after fingerprinting")

	// 5. (Optional but good) Verify reading starts from the correct place
	go tailer.readForever()
	tailer.decoder.Start()
	go tailer.forwardMessages()

	msg := <-tailer.outputChan
	suite.Equal("line3", string(msg.GetContent()))
}

func (suite *FingerprintTestSuite) createTailerWithConfig(fpConfig *logsconfig.FingerprintConfig) *Tailer {
	source := sources.NewReplaceableSource(sources.NewLogSource("", &config.LogsConfig{
		Type: config.FileType,
		Path: suite.testPath,
	}))

	info := status.NewInfoRegistry()
	tailerOptions := &TailerOptions{
		OutputChan:      make(chan *message.Message, 10),
		File:            NewFile(suite.testPath, source.UnderlyingSource(), false),
		SleepDuration:   10 * time.Millisecond,
		Decoder:         decoder.NewDecoderFromSource(source, info),
		Info:            info,
		PipelineMonitor: metrics.NewNoopPipelineMonitor(""),
	}

	tailer := NewTailer(tailerOptions)
	tailer.fingerprintConfig = fpConfig
	return tailer
}

func (suite *FingerprintTestSuite) TestLineBased_WithSkip1() {
	// Write test data
	lines := []string{
		"header line\n",
		"first data line\n",
		"second data line\n",
		"third data line\n",
	}

	for _, line := range lines {
		_, err := suite.testFile.WriteString(line)
		suite.Nil(err)
	}
	suite.testFile.Sync()

	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)

	maxLines := 2
	maxBytes := 1024
	linesToSkip := 1
	bytesToSkip := 0

	config := &logsconfig.FingerprintConfig{
		MaxLines:    &maxLines,
		MaxBytes:    &maxBytes,
		LinesToSkip: &linesToSkip,
		BytesToSkip: &bytesToSkip,
	}

	text := "first data line\nsecond data line\n"
	table := crc64.MakeTable(crc64.ISO)
	expectedChecksum := crc64.Checksum([]byte(text), table)

	tailer := suite.createTailerWithConfig(config)
	tailer.osFile = osFile

	receivedChecksum := tailer.ComputeFingerPrint()
	suite.Equal(expectedChecksum, receivedChecksum)
}

func (suite *FingerprintTestSuite) TestLineBased_SingleLongLine() {
	// Create a single line that exceeds maxBytes; thus we should only hash up to maxBytes
	longLine := make([]byte, 3000)
	for i := range longLine {
		longLine[i] = 'A'
	}
	longLine[2999] = '\n'

	_, err := suite.testFile.Write(longLine)
	suite.Nil(err)
	suite.testFile.Sync()

	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()

	maxLines := 5
	maxBytes := 2048
	linesToSkip := 0
	bytesToSkip := 0

	config := &logsconfig.FingerprintConfig{
		MaxLines:    &maxLines,
		MaxBytes:    &maxBytes,
		LinesToSkip: &linesToSkip,
		BytesToSkip: &bytesToSkip,
	}

	// Expected: line should be cut off and hashed up to maxBytes (2048)
	expectedText := make([]byte, 2048)
	for i := range expectedText {
		expectedText[i] = 'A'
	}

	table := crc64.MakeTable(crc64.ISO)
	expectedChecksum := crc64.Checksum(expectedText, table)

	tailer := suite.createTailerWithConfig(config)
	tailer.osFile = osFile

	receivedChecksum := tailer.ComputeFingerPrint()
	suite.Equal(expectedChecksum, receivedChecksum)
}

func (suite *FingerprintTestSuite) TestLineBased_MultipleLinesAddUpToByteLimit() {
	// Write multiple lines that together exceed maxBytes; thus we should only hash up to maxBytes
	line1Content := string(make([]byte, 800))
	line2Content := string(make([]byte, 800))
	line3Content := string(make([]byte, 800))

	lines := []string{
		"line1: " + line1Content + "\n",
		"line2: " + line2Content + "\n",
		"line3: " + line3Content + "\n",
	}

	for _, line := range lines {
		_, err := suite.testFile.WriteString(line)
		suite.Nil(err)
	}
	suite.testFile.Sync()

	fmt.Println(lines)
	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()
	maxLines := 10
	maxBytes := 1500
	linesToSkip := 0
	bytesToSkip := 0

	config := &logsconfig.FingerprintConfig{
		MaxLines:    &maxLines,
		MaxBytes:    &maxBytes,
		LinesToSkip: &linesToSkip,
		BytesToSkip: &bytesToSkip,
	}

	//Should hash up to maxBytes (1500) which includes the 807 bytes of line1 and the first 693 bytes of line2 (which accounts for the text "line1: ", "line2: ", and the line break)
	expectedText := "line1: " + line1Content + "\n" + "line2: " + line2Content[:685]

	table := crc64.MakeTable(crc64.ISO)
	fmt.Println(len(expectedText))
	expectedChecksum := crc64.Checksum([]byte(expectedText), table)

	tailer := suite.createTailerWithConfig(config)
	tailer.osFile = osFile

	receivedChecksum := tailer.ComputeFingerPrint()
	suite.Equal(expectedChecksum, receivedChecksum)
}

func (suite *FingerprintTestSuite) TestLineBased_WithSkip2() {
	lines := []string{
		"skip1\n",
		"skip2\n",
		"keep1\n",
		"keep2\n",
	}

	for _, line := range lines {
		_, err := suite.testFile.WriteString(line)
		suite.Nil(err)
	}
	suite.testFile.Sync()

	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()

	maxLines := 2
	maxBytes := 1024
	linesToSkip := 2
	bytesToSkip := 0

	config := &logsconfig.FingerprintConfig{
		MaxLines:    &maxLines,
		MaxBytes:    &maxBytes,
		LinesToSkip: &linesToSkip,
		BytesToSkip: &bytesToSkip,
	}
	// Expected: skip "skip1\n" and "skip2\n", then fingerprint "keep1\n" and "keep2\n"
	expectedText := "keep1\nkeep2\n"

	table := crc64.MakeTable(crc64.ISO)
	expectedChecksum := crc64.Checksum([]byte(expectedText), table)

	tailer := suite.createTailerWithConfig(config)
	tailer.osFile = osFile

	receivedChecksum := tailer.ComputeFingerPrint()
	suite.Equal(expectedChecksum, receivedChecksum)
}

func (suite *FingerprintTestSuite) TestLineBased_EmptyFile() {
	// Don't write anything to the file
	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()

	maxLines := 5
	maxBytes := 1024
	linesToSkip := 0
	bytesToSkip := 0

	config := &logsconfig.FingerprintConfig{
		MaxLines:    &maxLines,
		MaxBytes:    &maxBytes,
		LinesToSkip: &linesToSkip,
		BytesToSkip: &bytesToSkip,
	}

	// Expected: empty file should return 0 since we don't have any data to hash
	expectedChecksum := uint64(0)

	tailer := suite.createTailerWithConfig(config)
	tailer.osFile = osFile

	receivedChecksum := tailer.ComputeFingerPrint()
	suite.Equal(expectedChecksum, receivedChecksum)
}

// We don't have enough data to hash with the maxLines we configured we have neither the appropriate number of lines or bytes
func (suite *FingerprintTestSuite) TestLineBased_InsufficientData() {
	// Write fewer lines than maxLines with not enough data to hash
	lines := []string{"line1\n", "line2\n"}

	for _, line := range lines {
		_, err := suite.testFile.WriteString(line)
		suite.Nil(err)
	}
	suite.testFile.Sync()

	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()

	maxLines := 5
	maxBytes := 1024
	linesToSkip := 0
	bytesToSkip := 0

	config := &logsconfig.FingerprintConfig{
		MaxLines:    &maxLines,
		MaxBytes:    &maxBytes,
		LinesToSkip: &linesToSkip,
		BytesToSkip: &bytesToSkip,
	}

	// Expected: should return 0 because we have fewer lines than maxLines and less than 1024 bytes
	expectedChecksum := uint64(0)

	tailer := suite.createTailerWithConfig(config)
	tailer.osFile = osFile

	receivedChecksum := tailer.ComputeFingerPrint()
	suite.Equal(expectedChecksum, receivedChecksum)
}

// Skip x bytes and hash the next y bytes
func (suite *FingerprintTestSuite) TestByteBased_WithSkip1() {
	data := "header data that should be skipped" +
		"this is the actual data we want to fingerprint for testing purposes"

	_, err := suite.testFile.WriteString(data)
	suite.Nil(err)
	suite.testFile.Sync()

	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()
	maxLines := 0
	maxBytes := 50
	linesToSkip := 0
	bytesToSkip := 34

	config := &logsconfig.FingerprintConfig{
		MaxLines:    &maxLines,
		MaxBytes:    &maxBytes,
		LinesToSkip: &linesToSkip,
		BytesToSkip: &bytesToSkip,
	}

	// Expected: skip first 34 bytes, then fingerprint next 50 bytes
	expectedText := "this is the actual data we want to fingerprint for"
	table := crc64.MakeTable(crc64.ISO)
	expectedChecksum := crc64.Checksum([]byte(expectedText), table)

	tailer := suite.createTailerWithConfig(config)
	tailer.osFile = osFile

	receivedChecksum := tailer.ComputeFingerPrint()
	suite.Equal(expectedChecksum, receivedChecksum)
}

// Skip x bytes but there is not enough data to hash with the remaining y bytes
func (suite *FingerprintTestSuite) TestByteBased_WithSkip_InvalidNotEnoughData() {
	data := "header data that should be skipped" +
		"this is the actual data we want to fingerprint for testing purposes"

	_, err := suite.testFile.WriteString(data)
	suite.Nil(err)
	suite.testFile.Sync()

	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()
	maxLines := 0
	maxBytes := 1000
	linesToSkip := 0
	bytesToSkip := 34

	config := &logsconfig.FingerprintConfig{
		MaxLines:    &maxLines,
		MaxBytes:    &maxBytes,
		LinesToSkip: &linesToSkip,
		BytesToSkip: &bytesToSkip,
	}

	// Expected: skip first 34 bytes, but unable to fingerprint since less than 1000 we configured
	expectedChecksum := uint64(0)

	tailer := suite.createTailerWithConfig(config)
	tailer.osFile = osFile

	receivedChecksum := tailer.ComputeFingerPrint()
	suite.Equal(expectedChecksum, receivedChecksum)
}

// Test byte-based fingerprinting functionality with no skip
func (suite *FingerprintTestSuite) TestByteBased_NoSkip() {
	data := "this data should be fingerprinted from the beginning"

	_, err := suite.testFile.WriteString(data)
	suite.Nil(err)
	suite.testFile.Sync()

	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()

	maxLines := 0
	maxBytes := 30
	linesToSkip := 0
	bytesToSkip := 0

	config := &logsconfig.FingerprintConfig{
		MaxLines:    &maxLines,
		MaxBytes:    &maxBytes,
		LinesToSkip: &linesToSkip,
		BytesToSkip: &bytesToSkip,
	}

	// Expected: fingerprint first 30 bytes
	expectedText := "this data should be fingerprin"

	table := crc64.MakeTable(crc64.ISO)
	expectedChecksum := crc64.Checksum([]byte(expectedText), table)

	tailer := suite.createTailerWithConfig(config)
	tailer.osFile = osFile

	receivedChecksum := tailer.ComputeFingerPrint()
	suite.Equal(expectedChecksum, receivedChecksum)
}

// We don't have enough data to hash with the maxBytes we configured
func (suite *FingerprintTestSuite) TestByteBased_InsufficientData() {
	data := "short"

	_, err := suite.testFile.WriteString(data)
	suite.Nil(err)
	suite.testFile.Sync()

	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()

	maxLines := 0
	maxBytes := 100
	linesToSkip := 0
	bytesToSkip := 0

	config := &logsconfig.FingerprintConfig{
		MaxLines:    &maxLines,
		MaxBytes:    &maxBytes,
		LinesToSkip: &linesToSkip,
		BytesToSkip: &bytesToSkip,
	}

	// Expected: should return 0 because we have less data than maxBytes
	expectedChecksum := uint64(0)

	tailer := suite.createTailerWithConfig(config)
	tailer.osFile = osFile

	receivedChecksum := tailer.ComputeFingerPrint()
	suite.Equal(expectedChecksum, receivedChecksum)
}

// Given our current config, we should skip the first line and hash the remaining lines
func (suite *FingerprintTestSuite) TestLineBased_WithSkip3() {
	// Write some test data
	lines := []string{"line1\n", "line2\n", "line3\n"}
	for _, line := range lines {
		_, err := suite.testFile.WriteString(line)
		suite.Nil(err)
	}
	suite.testFile.Sync()

	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()

	maxLines := 2
	maxBytes := 1024
	linesToSkip := 1
	bytesToSkip := 0

	config := &logsconfig.FingerprintConfig{
		MaxLines:    &maxLines,
		MaxBytes:    &maxBytes,
		LinesToSkip: &linesToSkip,
		BytesToSkip: &bytesToSkip,
	}

	// Expected: skip first line, then fingerprint remaining lines
	expectedText := "line2\nline3\n"

	table := crc64.MakeTable(crc64.ISO)
	expectedChecksum := crc64.Checksum([]byte(expectedText), table)

	tailer := suite.createTailerWithConfig(config)
	tailer.osFile = osFile

	receivedChecksum := tailer.ComputeFingerPrint()
	suite.Equal(expectedChecksum, receivedChecksum)
}

// Given our current config, we should infer the user wants to fingerprint using bytes even though there is a maxLines in the config
func (suite *FingerprintTestSuite) TestByteBased_WithSkip2() {
	// Write some test data
	data := "this is test data for byte mode"
	_, err := suite.testFile.WriteString(data)
	suite.Nil(err)
	suite.testFile.Sync()

	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()

	maxLines := 5
	maxBytes := 21
	linesToSkip := 0
	bytesToSkip := 10

	config := &logsconfig.FingerprintConfig{
		MaxLines:    &maxLines,
		MaxBytes:    &maxBytes,
		LinesToSkip: &linesToSkip,
		BytesToSkip: &bytesToSkip,
	}
	expectedText := "st data for byte mode"
	table := crc64.MakeTable(crc64.ISO)
	expectedChecksum := crc64.Checksum([]byte(expectedText), table)

	tailer := suite.createTailerWithConfig(config)
	tailer.osFile = osFile

	receivedChecksum := tailer.ComputeFingerPrint()
	suite.Equal(expectedChecksum, receivedChecksum)
}

// Given our current config, we should default or infer the user wants to hash lines (3 to be exact)
func (suite *FingerprintTestSuite) TestLineBased_NoSkip() {
	lines := []string{"line1\n", "line2\n", "line3\n"}
	for _, line := range lines {
		_, err := suite.testFile.WriteString(line)
		suite.Nil(err)
	}
	suite.testFile.Sync()

	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()

	maxLines := 3
	maxBytes := 1024
	linesToSkip := 0
	bytesToSkip := 0

	config := &logsconfig.FingerprintConfig{
		MaxLines:    &maxLines,
		MaxBytes:    &maxBytes,
		LinesToSkip: &linesToSkip,
		BytesToSkip: &bytesToSkip,
	}

	// Expected: should fingerprint all lines
	expectedText := "line1\nline2\nline3\n"

	table := crc64.MakeTable(crc64.ISO)
	expectedChecksum := crc64.Checksum([]byte(expectedText), table)

	tailer := suite.createTailerWithConfig(config)
	tailer.osFile = osFile

	receivedChecksum := tailer.ComputeFingerPrint()
	suite.Equal(expectedChecksum, receivedChecksum)
}

func (suite *FingerprintTestSuite) TestLineBased_WithSkip5() {
	// Write test data
	lines := []string{
		"skip this header line\n",
		"line 1: important data\n",
		"line 2: more important data\n",
	}

	for _, line := range lines {
		_, err := suite.testFile.WriteString(line)
		suite.Nil(err)
	}
	suite.testFile.Sync()

	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()

	maxLines := 2
	maxBytes := 1024
	linesToSkip := 1
	bytesToSkip := 0

	config := &logsconfig.FingerprintConfig{
		MaxLines:    &maxLines,
		MaxBytes:    &maxBytes,
		LinesToSkip: &linesToSkip,
		BytesToSkip: &bytesToSkip,
	}

	tailer := suite.createTailerWithConfig(config)
	tailer.osFile = osFile

	// Compute fingerprint (now returns uint64 directly)
	fingerprint := tailer.ComputeFingerPrint()

	expectedText := "line 1: important data\n" + "line 2: more important data\n"
	table := crc64.MakeTable(crc64.ISO)
	expectedFingerprint := crc64.Checksum([]byte(expectedText), table)

	// Verify it's not zero (meaning it was computed successfully)
	suite.Equal(expectedFingerprint, fingerprint)
}

// Skips header info in bytes and hashes the rest of the data
func (suite *FingerprintTestSuite) TestByteBased_WithSkip3() {
	data := "SKIP_THIS_PART" + "thisisexactly20chars"

	_, err := suite.testFile.WriteString(data)
	suite.Nil(err)
	suite.testFile.Sync()

	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()

	maxLines := 0
	maxBytes := 20
	linesToSkip := 0
	bytesToSkip := 14

	config := &logsconfig.FingerprintConfig{
		MaxLines:    &maxLines,
		MaxBytes:    &maxBytes,
		LinesToSkip: &linesToSkip,
		BytesToSkip: &bytesToSkip,
	}

	tailer := suite.createTailerWithConfig(config)
	tailer.osFile = osFile
	fingerprint := tailer.ComputeFingerPrint()

	textToHash := "thisisexactly20chars"
	table := crc64.MakeTable(crc64.ISO)
	expectedHash := crc64.Checksum([]byte(textToHash), table)

	suite.Equal(expectedHash, fingerprint)
}

func (suite *FingerprintTestSuite) TestEmptyFile_And_SkippingMoreThanFileSize() {
	// Test 1: Empty file
	maxLines := 5
	maxBytes := 1024
	linesToSkip := 0
	bytesToSkip := 0
	config := &logsconfig.FingerprintConfig{
		MaxLines:    &maxLines,
		MaxBytes:    &maxBytes,
		LinesToSkip: &linesToSkip,
		BytesToSkip: &bytesToSkip,
	}

	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)

	tailer := suite.createTailerWithConfig(config)
	tailer.osFile = osFile
	fingerprint := tailer.ComputeFingerPrint()
	suite.Equal(uint64(0), fingerprint, "Empty file should return 0")
	osFile.Close()

	// Test 2: Insufficient data after skipping
	_, err = suite.testFile.WriteString("short")
	suite.Nil(err)
	suite.testFile.Sync()

	bytesToSkip = 10 // More than file size
	config.BytesToSkip = &bytesToSkip

	osFile, err = os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()

	tailer = suite.createTailerWithConfig(config)
	tailer.osFile = osFile
	fingerprint = tailer.ComputeFingerPrint()
	suite.Equal(uint64(0), fingerprint, "Insufficient data should return 0")
}

func (suite *FingerprintTestSuite) TestLineBased_SingleLongLine2() {
	// Create a line longer than maxBytes to test truncation
	longContent := strings.Repeat("X", 80) + strings.Repeat("Y", 80)
	longLine := longContent + "\n"

	_, err := suite.testFile.WriteString(longLine)
	suite.Nil(err)
	suite.testFile.Sync()

	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()

	maxLines := 1
	maxBytes := 80
	linesToSkip := 0
	bytesToSkip := 0

	config := &logsconfig.FingerprintConfig{
		MaxLines:    &maxLines,
		MaxBytes:    &maxBytes,
		LinesToSkip: &linesToSkip,
		BytesToSkip: &bytesToSkip,
	}

	tailer := suite.createTailerWithConfig(config)
	tailer.osFile = osFile
	fingerprint := tailer.ComputeFingerPrint()

	expectedText := strings.Repeat("X", 80)
	table := crc64.MakeTable(crc64.ISO)
	expectedHash := crc64.Checksum([]byte(expectedText), table)
	// Should still compute a fingerprint even with truncation
	suite.Equal(expectedHash, fingerprint)
}

// Tests the "whichever comes first" logic (X lines or Y bytes)
func (suite *FingerprintTestSuite) TestXLinesOrYBytesFirstHash() {
	lines := []string{
		strings.Repeat("A", 30) + "\n", // ~31 bytes
		strings.Repeat("B", 30) + "\n", // ~31 bytes
		strings.Repeat("C", 30) + "\n", // ~31 bytes
		strings.Repeat("D", 30) + "\n", // ~31 bytes
	}

	for _, line := range lines {
		_, err := suite.testFile.WriteString(line)
		suite.Nil(err)
	}
	suite.testFile.Sync()

	maxLines := 4
	maxBytes := 80
	linesToSkip := 0
	bytesToSkip := 0

	config := &logsconfig.FingerprintConfig{
		MaxLines:    &maxLines,
		MaxBytes:    &maxBytes,
		LinesToSkip: &linesToSkip,
		BytesToSkip: &bytesToSkip,
	}

	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()

	tailer := suite.createTailerWithConfig(config)
	tailer.osFile = osFile
	fingerprint := tailer.ComputeFingerPrint()

	fmt.Println(lines)
	stringToHash := strings.Repeat("A", 30) + "\n" + strings.Repeat("B", 30) + "\n" + strings.Repeat("C", 18)
	table := crc64.MakeTable(crc64.ISO)
	expectedHash := crc64.Checksum([]byte(stringToHash), table)

	suite.Equal(expectedHash, fingerprint)
}
func (suite *FingerprintTestSuite) TestLineBased_WithSkip4() {
	data := "line1\nline2\nline3\n"

	// Test line mode selection
	_, err := suite.testFile.WriteString(data)
	suite.Nil(err)
	suite.testFile.Sync()

	maxLines := 2
	maxBytes := 1024
	linesToSkip := 1
	bytesToSkip := 0

	config := &logsconfig.FingerprintConfig{
		MaxLines:    &maxLines,
		MaxBytes:    &maxBytes,
		LinesToSkip: &linesToSkip,
		BytesToSkip: &bytesToSkip,
	}

	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)

	tailer := suite.createTailerWithConfig(config)
	tailer.osFile = osFile
	fingerprint1 := tailer.ComputeFingerPrint()
	osFile.Close()

	// Reset file for next test
	suite.testFile.Seek(0, 0)
	suite.testFile.Truncate(0)
	_, err = suite.testFile.WriteString(data)
	suite.Nil(err)
	suite.testFile.Sync()

	textToHash1 := "line2\nline3\n"
	table := crc64.MakeTable(crc64.ISO)
	expectedHash1 := crc64.Checksum([]byte(textToHash1), table)
	suite.Equal(fingerprint1, expectedHash1)

	maxLines = 2
	maxBytes = 10
	linesToSkip = 0
	bytesToSkip = 5

	config = &logsconfig.FingerprintConfig{
		MaxLines:    &maxLines,
		MaxBytes:    &maxBytes,
		LinesToSkip: &linesToSkip,
		BytesToSkip: &bytesToSkip,
	}

	osFile, err = os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()

	tailer = suite.createTailerWithConfig(config)
	tailer.osFile = osFile
	fingerprint2 := tailer.ComputeFingerPrint()
	textToHash2 := "\nline2\nlin"
	table = crc64.MakeTable(crc64.ISO)
	expectedHash2 := crc64.Checksum([]byte(textToHash2), table)
	suite.Equal(expectedHash2, fingerprint2)
}

func (suite *FingerprintTestSuite) TestInvalidConfig_BothSkipValuesSet() {
	// This test handles when bytesToSkip and linesToSkip are non-zero,
	// the content to hash is a single line, and this line is longer than maxBytes.
	// Because linesToSkip > 0, we operate in line-mode. The tailer should skip the
	// specified number of lines, read the next line, truncate it to maxBytes, and hash that.

	lines := []string{
		"this line should be skipped\n",
	}

	for _, line := range lines {
		_, err := suite.testFile.WriteString(line)
		suite.Nil(err)
	}
	suite.testFile.Sync()

	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()
	//invalid config
	maxLines := 1
	maxBytes := 4
	linesToSkip := 1
	bytesToSkip := 1

	config := &logsconfig.FingerprintConfig{
		MaxLines:    &maxLines,
		MaxBytes:    &maxBytes,
		LinesToSkip: &linesToSkip,
		BytesToSkip: &bytesToSkip,
	}

	// Expected: skip the first line. Read the second line, but only up to maxBytes.

	tailer := suite.createTailerWithConfig(config)
	tailer.osFile = osFile

	expectedChecksum := uint64(0)
	receivedChecksum := tailer.ComputeFingerPrint()

	suite.Equal(expectedChecksum, receivedChecksum)
}

// Tests whether or not rotation was accurately detected
func (suite *FingerprintTestSuite) TestDidRotateViaFingerprint() {
	// 1. Start with a file with content and create a tailer.
	suite.T().Log("Writing initial content and creating tailer")
	_, err := suite.testFile.WriteString("line 1\nline 2\nline 3\n")
	suite.Nil(err)
	suite.Nil(suite.testFile.Sync())
	mockConfig := configmock.New(suite.T())
	mockConfig.SetWithoutSource("logs_config.fingerprint_strategy", "checksum")
	mockConfig.SetWithoutSource("logs_config.fingerprint_config.maxlines", 1)
	mockConfig.SetWithoutSource("logs_config.fingerprint_config.maxbytes", 2048)
	mockConfig.SetWithoutSource("logs_config.fingerprint_config.bytes_to_skip", 0)
	mockConfig.SetWithoutSource("logs_config.fingerprint_config.lines_to_skip", 0)
	config := returnFingerprintConfig()
	suite.Equal(1, *config.MaxLines)
	suite.Equal(2048, *config.MaxBytes)
	suite.Equal(0, *config.BytesToSkip)
	suite.Equal(0, *config.LinesToSkip)
	tailer := suite.createTailerWithConfig(config)
	tailer.fingerprintingEnabled = true
	tailer.fingerprint = tailer.ComputeFingerPrint()

	table := crc64.MakeTable(crc64.ISO)
	expectedChecksum := crc64.Checksum([]byte("line 1\n"), table)
	suite.Equal(expectedChecksum, tailer.fingerprint)

	// 2. Immediately check for rotation. It should be false as the file is unchanged.
	suite.T().Log("Checking for rotation on unchanged file")
	rotated, err := tailer.DidRotateViaFingerprint()
	suite.Nil(err)
	suite.False(rotated, "Should not detect rotation on an unchanged file")

	// 3. Truncate the file, which simulates a rotation. This should be detected.
	suite.T().Log("Truncating file to simulate rotation")
	suite.Nil(suite.testFile.Truncate(0))
	_, err = suite.testFile.Seek(0, 0)
	suite.Nil(err)
	suite.Nil(suite.testFile.Sync())
	rotated, err = tailer.DidRotateViaFingerprint()
	suite.Nil(err)
	suite.True(rotated, "Should detect rotation after truncation")

	// 4. Simulate a full file replacement (e.g. logrotate with 'create' directive).
	suite.T().Log("Simulating file replacement with different content")
	_, err = suite.testFile.WriteString("a completely new file\n")
	suite.Nil(err)
	suite.Nil(suite.testFile.Sync())

	// We 're-arm' the tailer, as if the launcher had picked up the new file.
	// This tailer now considers the current content ("a completely new file") as its baseline.
	tailer = suite.createTailerWithConfig(config)
	tailer.fingerprintingEnabled = true
	tailer.fingerprint = tailer.ComputeFingerPrint()

	expectedChecksum = crc64.Checksum([]byte("a completely new file\n"), table)
	suite.Equal(expectedChecksum, tailer.fingerprint)

	// Check for rotation immediately after re-arming. Since the file hasn't changed
	// since the tailer was created, it should report no rotation. Its internal fingerprint
	// matches the file's current fingerprint.
	rotated, err = tailer.DidRotateViaFingerprint()
	suite.Nil(err)
	suite.False(rotated, "Should not detect rotation immediately after creating a new tailer on a file")

	expectedChecksum = crc64.Checksum([]byte("a completely new file\n"), table)
	suite.Equal(expectedChecksum, tailer.fingerprint)

	// Now, modify the file again. This change *should* be detected as a rotation.
	suite.T().Log("Simulating another rotation on the new file")
	suite.Nil(suite.testFile.Truncate(0))
	_, err = suite.testFile.Seek(0, 0)
	suite.Nil(err)
	_, err = suite.testFile.WriteString("even more different content\n")
	suite.Nil(err)
	suite.Nil(suite.testFile.Sync())

	rotated, err = tailer.DidRotateViaFingerprint()
	suite.Nil(err)
	suite.True(rotated, "Should detect rotation after file content changes")
	expectedChecksum = crc64.Checksum([]byte("even more different content\n"), table)
	receivedChecksum := tailer.ComputeFingerPrint()
	suite.Equal(expectedChecksum, receivedChecksum)

	// 5. Test case with an an empty file.
	// The initial fingerprint will be 0.
	suite.T().Log("Testing rotation detection with an initially empty file")
	suite.Nil(suite.testFile.Truncate(0))
	_, err = suite.testFile.Seek(0, 0)
	suite.Nil(err)
	suite.Nil(suite.testFile.Sync())
	tailer = suite.createTailerWithConfig(config)
	tailer.fingerprintingEnabled = true
	tailer.fingerprint = tailer.ComputeFingerPrint()
	suite.Zero(tailer.fingerprint, "Fingerprint of an empty file should be 0")

	// `DidRotateViaFingerprint` is designed to return `false` if the original
	// fingerprint was 0, to avoid false positives.
	rotated, err = tailer.DidRotateViaFingerprint()
	suite.Nil(err)
	suite.False(rotated, "Should not detect rotation if the initial fingerprint was zero")
	suite.Equal(uint64(0), tailer.ComputeFingerPrint())
}
