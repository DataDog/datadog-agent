// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/goleak"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
)

var chanSize = 10
var closeTimeout = 1 * time.Second

// TailerTestSuite contains unit tests for the file tailer.
// These tests are focused on verifying the core functionality of the file tailer
// with minimal external dependencies. The goal moving forward is to move
// all of these tests over to file mocks or the integration test suite.
type TailerTestSuite struct {
	suite.Suite
	testDir  string
	testPath string
	testFile *os.File

	tailer     *Tailer
	outputChan chan *message.Message
	source     *sources.ReplaceableSource
}

// createTailerOptions creates TailerOptions with common defaults.
// Parameters that vary between tests can be customized via the opts parameter.
type tailerTestOptions struct {
	source     *sources.LogSource
	isWildcard bool
}

func (suite *TailerTestSuite) createTailerOptions(opts *tailerTestOptions) *TailerOptions {
	if opts == nil {
		opts = &tailerTestOptions{}
	}

	// Default to suite.source if no source provided
	source := opts.source
	if source == nil {
		source = suite.source.UnderlyingSource()
	}

	sleepDuration := 10 * time.Millisecond
	info := status.NewInfoRegistry()

	return &TailerOptions{
		OutputChan:      suite.outputChan,
		File:            NewFile(suite.testPath, source, opts.isWildcard),
		SleepDuration:   sleepDuration,
		Decoder:         decoder.NewDecoderFromSource(suite.source, info),
		Info:            info,
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
		Registry:        auditor.NewMockRegistry(),
		FileOpener:      opener.NewFileOpener(),
	}
}

func TestSuite(t *testing.T) {
	suite.Run(t, new(TailerTestSuite))
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

	suite.tailer = NewTailer(suite.createTailerOptions(nil))
	suite.tailer.closeTimeout = closeTimeout
}

func (suite *TailerTestSuite) TearDownTest() {
	suite.tailer.Stop()
	suite.testFile.Close()
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

	mockConfig.SetInTest("logs_config.close_timeout", 42)

	tailer := NewTailer(suite.createTailerOptions(nil))
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

func (suite *TailerTestSuite) TestWithBlanklinesSingleLineHandler() {
	mockConfig := configmock.New(suite.T())
	mockConfig.SetInTest("logs_config.auto_multi_line_detection_tagging", false)

	// Recreate the tailer after config change so decoder uses SingleLineHandler
	suite.tailer = NewTailer(suite.createTailerOptions(nil))

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
		"file:"+filepath.Join(suite.testDir, "tailer.log"),
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

	suite.tailer = NewTailer(suite.createTailerOptions(&tailerTestOptions{
		source:     dirTaggedSource,
		isWildcard: true,
	}))
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

	suite.tailer = NewTailer(suite.createTailerOptions(&tailerTestOptions{
		source:     dirTaggedSource,
		isWildcard: false,
	}))

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

	suite.tailer = NewTailer(suite.createTailerOptions(&tailerTestOptions{
		source:     dirTaggedSource,
		isWildcard: true,
	}))
	suite.tailer.StartFromBeginning()

	tags := suite.tailer.buildTailerTags()
	suite.ElementsMatch([]string{
		"filename:" + filepath.Base(suite.testFile.Name()),
		"dirname:" + filepath.Dir(suite.testFile.Name()),
	}, tags)
}

func (suite *TailerTestSuite) TestTruncatedTagAutoMultilineHandler() {
	mockConfig := configmock.New(suite.T())
	mockConfig.SetInTest("logs_config.max_message_size_bytes", 100)     // Small size to force truncation when aggregated
	mockConfig.SetInTest("logs_config.tag_truncated_logs", true)        // Enable truncation tagging
	mockConfig.SetInTest("logs_config.tag_multi_line_logs", true)       // Enable multiline tagging
	mockConfig.SetInTest("logs_config.auto_multi_line_detection", true) // Enable multiline tagging

	// Enable auto multiline detection with aggregation (not just detection-only tagging)
	mockConfig.SetInTest("logs_config.auto_multi_line_detection_tagging", false) // Disable detection-only
	// Instead, enable full auto multiline on the source itself

	defer mockConfig.SetInTest("logs_config.max_message_size_bytes", pkgconfigsetup.DefaultMaxMessageSizeBytes)
	defer mockConfig.SetInTest("logs_config.tag_truncated_logs", false)
	defer mockConfig.SetInTest("logs_config.tag_multi_line_logs", false)

	autoML := true
	source := sources.NewLogSource("", &config.LogsConfig{
		Type:          config.FileType,
		Path:          suite.testPath,
		AutoMultiLine: &autoML, // Enable auto multiline aggregation
	})

	suite.tailer = NewTailer(suite.createTailerOptions(&tailerTestOptions{
		source:     source,
		isWildcard: true,
	}))
	suite.tailer.StartFromBeginning()

	// Write multiline logs that will exceed the size limit when combined
	// Use a recognized timestamp format with time component
	// Line 1: ~60 bytes, Line 2: ~50 bytes, Combined: ~112 bytes (exceeds 100 byte limit)
	_, err := suite.testFile.WriteString("2024-01-01 10:00:00 [ERROR] First line of multiline log message\n")
	suite.Nil(err)
	_, err = suite.testFile.WriteString("  continuation line that should be aggregated here\n") // This should be aggregated with the first line
	suite.Nil(err)
	// Write a new log with timestamp to trigger flush of the previous multiline group
	_, err = suite.testFile.WriteString("2024-01-01 10:00:01 [INFO] Next log\n")
	suite.Nil(err)

	// First message should be the aggregated multiline log with truncation
	msg := <-suite.outputChan
	tags := msg.Tags()

	// Check for truncation tag with "auto_multiline" reason
	suite.Contains(tags, message.TruncatedReasonTag("auto_multiline"))

	// The content should contain the truncation flag
	content := string(msg.GetContent())
	suite.Contains(content, "...TRUNCATED...")

	// Second message should be the single-line log
	msg2 := <-suite.outputChan
	suite.NotNil(msg2)
}

func (suite *TailerTestSuite) TestTruncatedTagSingleLineHandler() {
	mockConfig := configmock.New(suite.T())
	mockConfig.SetInTest("logs_config.max_message_size_bytes", 3)
	mockConfig.SetInTest("logs_config.tag_truncated_logs", true)
	mockConfig.SetInTest("logs_config.auto_multi_line_detection_tagging", false)
	defer mockConfig.SetInTest("logs_config.max_message_size_bytes", pkgconfigsetup.DefaultMaxMessageSizeBytes)
	defer mockConfig.SetInTest("logs_config.tag_truncated_logs", false)
	defer mockConfig.SetInTest("logs_config.auto_multi_line_detection_tagging", true)

	source := sources.NewLogSource("", &config.LogsConfig{
		Type: config.FileType,
		Path: suite.testPath,
	})

	suite.tailer = NewTailer(suite.createTailerOptions(&tailerTestOptions{
		source:     source,
		isWildcard: true,
	}))
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

	suite.tailer = NewTailer(suite.createTailerOptions(&tailerTestOptions{
		isWildcard: true,
	}))

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

	tailer := NewTailer(suite.createTailerOptions(nil))
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

// Test_RotationThenShutdownNoGoroutineLeak tests the following scenario:
//  1. File rotation is detected => StopAfterFileRotation() called (goroutine sleeps)
//  2. Agent shutdown happens => Stop() called on the rotated tailer
//  3. Stop() signals channel and waits for completion
//  4. StopAfterFileRotation goroutine wakes up and tries to send
//     to validate that if there is a race condition, the goroutine will exit cleanly
func TestNoGoLeakWithNonBlockingStop(t *testing.T) {
	// Ignore all goroutines that exist before the test starts (background workers from logging, caching, etc.)
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	testDir := t.TempDir()
	testPath := filepath.Join(testDir, "tailer.log")
	f, err := os.Create(testPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	outputChan := make(chan *message.Message, chanSize)
	source := sources.NewReplaceableSource(sources.NewLogSource("", &config.LogsConfig{
		Type: config.FileType,
		Path: testPath,
	}))
	sleepDuration := 10 * time.Millisecond
	info := status.NewInfoRegistry()

	tailerOptions := &TailerOptions{
		OutputChan:      outputChan,
		File:            NewFile(testPath, source.UnderlyingSource(), false),
		SleepDuration:   sleepDuration,
		Decoder:         decoder.NewDecoderFromSource(source, info),
		Info:            info,
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
		Registry:        auditor.NewMockRegistry(),
		FileOpener:      opener.NewFileOpener(),
	}

	tailer := NewTailer(tailerOptions)
	tailer.closeTimeout = 20 * time.Millisecond // Short timeout for test

	// Write some data and start tailer
	_, err = f.WriteString("line 1\nline 2\n")
	if err != nil {
		t.Fatal(err)
	}

	err = tailer.StartFromBeginning()
	if err != nil {
		t.Fatal(err)
	}

	// Drain messages
	<-outputChan
	<-outputChan

	// ROTATION DETECTED ...
	// StopAfterFileRotation spawns goroutine that sleeps for closeTimeout, tries to send to the stop channel
	tailer.StopAfterFileRotation()

	// RN...
	// - goroutine is sleeping for closeTimeout
	// - The tailer is still running (readForever is active)

	// Sleep briefly to make sure the goroutine is actually sleeping
	time.Sleep(10 * time.Millisecond)

	// Stop() is called on the rotated tailer (simulating launcher.cleanup())
	// This will signal the stop channel, readForever drains it and exits, forwardMessages finishes and closes done channel, Stop() returns after <-t.done
	tailer.Stop()

	// RN...
	// - tailer is fully stopped (readForever exited, done channel closed)
	// - stop channel is empty (0/1)
	// - StopAfterFileRotation goroutine is still sleeping (not woken up yet)

	// Wait for the closeTimeout to expire
	// The StopAfterFileRotation goroutine will wake up and try to send to the stop channel,
	// but since readForever has already exited, there's no reader.
	// The select/default in StopAfterFileRotation will hit the default case, allowing the goroutine to exit cleanly.

	// Wait long enough for the goroutine to wake up and complete
	// closeTimeout is 20ms, so 100ms gives us plenty of buffer for slow CI machines
	time.Sleep(100 * time.Millisecond)

	// The deferred goleak.VerifyNone() will detect if goroutine leaked
}
