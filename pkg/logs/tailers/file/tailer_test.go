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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/goleak"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
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

func (suite *TailerTestSuite) TestTailerRotationHandoffQuietPeriodConfig() {
	mockConfig := configmock.New(suite.T())
	// To satisfy the suite level tailer
	suite.tailer.StartFromBeginning()

	tailer := NewTailer(suite.createTailerOptions(nil))
	suite.Equal(2*time.Second, tailer.rotationHandoffQuietPeriod)

	mockConfig.SetInTest("logs_config.rotation_handoff_quiet_period", 7)

	tailer = NewTailer(suite.createTailerOptions(nil))
	tailer.StartFromBeginning()

	suite.Equal(7*time.Second, tailer.rotationHandoffQuietPeriod)
	tailer.Stop()
}

// The setting is undocumented, so it is absent from the generated config
// templates and the env var is the only way an operator can reach it.
func TestRotationHandoffQuietPeriodEnvVar(t *testing.T) {
	t.Setenv("DD_LOGS_CONFIG_ROTATION_HANDOFF_QUIET_PERIOD", "9")

	quietPeriod := configmock.New(t).GetDuration("logs_config.rotation_handoff_quiet_period") * time.Second
	if quietPeriod != 9*time.Second {
		t.Errorf("read a quiet period of %s from the environment, expected %s", quietPeriod, 9*time.Second)
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

	defer mockConfig.SetInTest("logs_config.max_message_size_bytes", constants.DefaultMaxMessageSizeBytes)
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

	// The overflowed multiline group should fall back to standalone messages.
	msg := <-suite.outputChan
	tags := msg.Tags()
	content := string(msg.GetContent())
	suite.Equal("2024-01-01 10:00:00 [ERROR] First line of multiline log message", content)
	suite.NotContains(tags, message.TruncatedReasonTag("auto_multiline"))
	suite.NotContains(tags, message.MultiLineSourceTag("auto_multiline"))

	msg2 := <-suite.outputChan
	tags2 := msg2.Tags()
	content2 := string(msg2.GetContent())
	suite.Equal("continuation line that should be aggregated here", content2)
	suite.NotContains(tags2, message.TruncatedReasonTag("auto_multiline"))
	suite.NotContains(tags2, message.MultiLineSourceTag("auto_multiline"))

	// Third message should be the next single-line log.
	msg3 := <-suite.outputChan
	suite.Equal("2024-01-01 10:00:01 [INFO] Next log", string(msg3.GetContent()))
}

func (suite *TailerTestSuite) TestTruncatedTagSingleLineHandler() {
	mockConfig := configmock.New(suite.T())
	mockConfig.SetInTest("logs_config.max_message_size_bytes", 3)
	mockConfig.SetInTest("logs_config.tag_truncated_logs", true)
	mockConfig.SetInTest("logs_config.auto_multi_line_detection_tagging", false)
	defer mockConfig.SetInTest("logs_config.max_message_size_bytes", constants.DefaultMaxMessageSizeBytes)
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

// TestStructuredMessagePreserved verifies that forwardMessages preserves
// StateStructured messages (produced by the syslog file parser) instead of
// re-wrapping them as StateUnstructured. The output message must carry the
// full structured content (syslog metadata) and have a properly populated origin.
func TestStructuredMessagePreserved(t *testing.T) {
	testDir := t.TempDir()
	testPath := filepath.Join(testDir, "syslog.log")
	f, err := os.Create(testPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	outputChan := make(chan *message.Message, chanSize)
	// attribute_parsing gates whether the syslog parser is installed at all
	// (IsAttributeParsingEnabled); without it the decoder uses the noop parser
	// and the message stays StateUnstructured. debug_attr_parsing gates the
	// structured JSON envelope so the parser renders the "message"/"syslog"
	// object this test asserts on.
	attributeParsing := true
	debugAttrParsing := true
	source := sources.NewReplaceableSource(sources.NewLogSource("syslog-test", &config.LogsConfig{
		Type:             config.FileType,
		Path:             testPath,
		Format:           config.SyslogFormat,
		AttributeParsing: &attributeParsing,
		DebugAttrParsing: &debugAttrParsing,
	}))
	info := status.NewInfoRegistry()

	tailerOptions := &TailerOptions{
		OutputChan:      outputChan,
		File:            NewFile(testPath, source.UnderlyingSource(), false),
		SleepDuration:   10 * time.Millisecond,
		Decoder:         decoder.NewDecoderFromSource(source, info),
		Info:            info,
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
		Registry:        auditor.NewMockRegistry(),
		FileOpener:      opener.NewFileOpener(),
	}

	tailer := NewTailer(tailerOptions)

	syslogLine := "<165>1 2024-01-15T10:30:00Z myhost myapp 1234 - - Hello structured world\n"
	_, err = f.WriteString(syslogLine)
	if err != nil {
		t.Fatal(err)
	}

	err = tailer.StartFromBeginning()
	if err != nil {
		t.Fatal(err)
	}
	defer tailer.Stop()

	select {
	case msg := <-outputChan:
		if msg.State != message.StateStructured {
			t.Fatalf("expected StateStructured (%d), got state %d", message.StateStructured, msg.State)
		}

		rendered, err := msg.Render()
		if err != nil {
			t.Fatalf("failed to render structured message: %v", err)
		}

		renderedStr := string(rendered)
		if !strings.Contains(renderedStr, `"syslog"`) {
			t.Errorf("rendered output missing syslog metadata: %s", renderedStr)
		}
		if !strings.Contains(renderedStr, `"message"`) {
			t.Errorf("rendered output missing message field: %s", renderedStr)
		}
		if !strings.Contains(renderedStr, "Hello structured world") {
			t.Errorf("rendered output missing message body: %s", renderedStr)
		}
		if !strings.Contains(renderedStr, "myapp") {
			t.Errorf("rendered output missing appname: %s", renderedStr)
		}

		if msg.Origin == nil {
			t.Fatal("message origin is nil")
		}
		if msg.Origin.FilePath != testPath {
			t.Errorf("expected origin FilePath %q, got %q", testPath, msg.Origin.FilePath)
		}
		if msg.Origin.Offset == "" {
			t.Error("expected non-empty origin Offset")
		}

		if msg.Origin.Source() != "" {
			t.Errorf("expected empty origin Source (syslog does not override source directly), got %q", msg.Origin.Source())
		}
		if msg.Origin.Service() != "" {
			t.Errorf("expected empty origin Service (syslog does not override service), got %q", msg.Origin.Service())
		}

	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for message")
	}
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

// newDrainTestTailer builds the minimal state waitForRotationDrain reads, so the
// drain timing can be exercised without a file or a running pipeline.
func newDrainTestTailer(closeTimeout, sleepDuration, quietPeriod time.Duration) *Tailer {
	return &Tailer{
		closeTimeout:               closeTimeout,
		sleepDuration:              sleepDuration,
		rotationHandoffQuietPeriod: quietPeriod,
		bytesRead:                  status.NewCountInfo("Bytes Read"),
	}
}

func TestWaitForRotationDrainEndsWhenFileGoesIdle(t *testing.T) {
	t.Parallel()

	quietPeriod := 40 * time.Millisecond
	// The close timeout is far larger than the quiet period, so a drain that
	// ends quickly can only have ended on the quiet period.
	tailer := newDrainTestTailer(30*time.Second, 20*time.Millisecond, quietPeriod)

	start := time.Now()
	timedOut := tailer.waitForRotationDrain(true)
	elapsed := time.Since(start)

	if timedOut {
		t.Errorf("drain reported a close timeout, expected it to report that the file went idle")
	}
	if elapsed < quietPeriod {
		t.Errorf("drain ended after %s, before the quiet period of %s elapsed", elapsed, quietPeriod)
	}
	if elapsed > 5*time.Second {
		t.Errorf("drain took %s, expected it to end on the quiet period rather than on the close timeout of %s",
			elapsed, tailer.closeTimeout)
	}
}

func TestWaitForRotationDrainKeepsReadingWhileFileProducesData(t *testing.T) {
	t.Parallel()

	pollInterval := 20 * time.Millisecond
	tailer := newDrainTestTailer(30*time.Second, pollInterval, 40*time.Millisecond)

	// Produce a byte far more often than the poll interval, so every poll during
	// this window observes growth and the quiet period never accumulates.
	producing := 300 * time.Millisecond
	lastByteAt := time.Now().Add(producing)
	go func() {
		for time.Now().Before(lastByteAt) {
			tailer.bytesRead.Add(1)
			time.Sleep(pollInterval / 10)
		}
	}()

	start := time.Now()
	timedOut := tailer.waitForRotationDrain(true)
	elapsed := time.Since(start)

	if timedOut {
		t.Errorf("drain reported a close timeout, expected it to report that the file went idle")
	}
	if elapsed < producing {
		t.Errorf("drain ended after %s, but the file was still producing data for %s", elapsed, producing)
	}
	if elapsed > producing+5*time.Second {
		t.Errorf("drain took %s, expected it to end a quiet period after the last byte", elapsed)
	}
}

func TestWaitForRotationDrainHonorsConfiguredQuietPeriod(t *testing.T) {
	t.Parallel()

	pollInterval := 20 * time.Millisecond
	shortQuietPeriod := 50 * time.Millisecond
	longQuietPeriod := time.Second

	// Both files are idle from the start, so the only thing that can separate
	// the two drains is the configured quiet period.
	start := time.Now()
	newDrainTestTailer(30*time.Second, pollInterval, shortQuietPeriod).waitForRotationDrain(true)
	shortElapsed := time.Since(start)

	start = time.Now()
	newDrainTestTailer(30*time.Second, pollInterval, longQuietPeriod).waitForRotationDrain(true)
	longElapsed := time.Since(start)

	if shortElapsed < shortQuietPeriod {
		t.Errorf("drain ended after %s, before the configured quiet period of %s", shortElapsed, shortQuietPeriod)
	}
	if shortElapsed > longQuietPeriod/2 {
		t.Errorf("a %s quiet period ended the drain after %s, close to the %s one; the setting does not appear to be honored",
			shortQuietPeriod, shortElapsed, longQuietPeriod)
	}
	if longElapsed < longQuietPeriod {
		t.Errorf("drain ended after %s, before the configured quiet period of %s", longElapsed, longQuietPeriod)
	}
}

func TestWaitForRotationDrainIsBoundedByCloseTimeout(t *testing.T) {
	t.Parallel()

	pollInterval := 20 * time.Millisecond
	tailer := newDrainTestTailer(300*time.Millisecond, pollInterval, 40*time.Millisecond)

	stopProducing := make(chan struct{})
	defer close(stopProducing)
	go func() {
		for {
			select {
			case <-stopProducing:
				return
			default:
				tailer.bytesRead.Add(1)
				time.Sleep(pollInterval / 10)
			}
		}
	}()

	start := time.Now()
	timedOut := tailer.waitForRotationDrain(true)
	elapsed := time.Since(start)

	if !timedOut {
		t.Errorf("drain reported that the file went idle, expected it to report the close timeout")
	}
	if elapsed < tailer.closeTimeout {
		t.Errorf("drain ended after %s, before the close timeout of %s", elapsed, tailer.closeTimeout)
	}
	if elapsed > tailer.closeTimeout+5*time.Second {
		t.Errorf("drain took %s, expected the close timeout of %s to bound a file that never goes idle",
			elapsed, tailer.closeTimeout)
	}
}

func TestWaitForRotationDrainCloseTimeoutOutranksQuietPeriod(t *testing.T) {
	t.Parallel()

	// The file is idle from the start, but the quiet period it would have to stay
	// idle for is longer than the close timeout, which still has the last word.
	tailer := newDrainTestTailer(300*time.Millisecond, 20*time.Millisecond, 30*time.Second)

	start := time.Now()
	timedOut := tailer.waitForRotationDrain(true)
	elapsed := time.Since(start)

	if !timedOut {
		t.Errorf("drain reported that the file went idle, expected it to report the close timeout")
	}
	if elapsed < tailer.closeTimeout {
		t.Errorf("drain ended after %s, before the close timeout of %s", elapsed, tailer.closeTimeout)
	}
	if elapsed > tailer.closeTimeout+5*time.Second {
		t.Errorf("drain took %s, expected the close timeout of %s to bound a quiet period of %s",
			elapsed, tailer.closeTimeout, tailer.rotationHandoffQuietPeriod)
	}
}

func TestWaitForRotationDrainWithoutQuietPeriodAlwaysWaitsCloseTimeout(t *testing.T) {
	t.Parallel()

	// A quiet period of zero turns the early exit off, so even an idle file has
	// to wait out the whole close timeout.
	tailer := newDrainTestTailer(300*time.Millisecond, 20*time.Millisecond, 0)

	start := time.Now()
	timedOut := tailer.waitForRotationDrain(true)
	elapsed := time.Since(start)

	if !timedOut {
		t.Errorf("drain reported that the file went idle, expected it to report the close timeout")
	}
	if elapsed < tailer.closeTimeout {
		t.Errorf("drain ended after %s, expected the full close timeout of %s even though the file was idle",
			elapsed, tailer.closeTimeout)
	}
	if elapsed > tailer.closeTimeout+5*time.Second {
		t.Errorf("drain took %s, expected roughly the close timeout of %s", elapsed, tailer.closeTimeout)
	}
}

func TestWaitForRotationDrainWithoutIdleEndAlwaysWaitsCloseTimeout(t *testing.T) {
	t.Parallel()

	// The file never produces anything, so a drain that does not end when idle
	// has to wait out the whole close timeout.
	tailer := newDrainTestTailer(300*time.Millisecond, 20*time.Millisecond, 40*time.Millisecond)

	start := time.Now()
	timedOut := tailer.waitForRotationDrain(false)
	elapsed := time.Since(start)

	if !timedOut {
		t.Errorf("drain reported that the file went idle, expected it to report the close timeout")
	}
	if elapsed < tailer.closeTimeout {
		t.Errorf("drain ended after %s, expected the full close timeout of %s even though the file was idle",
			elapsed, tailer.closeTimeout)
	}
	if elapsed > tailer.closeTimeout+5*time.Second {
		t.Errorf("drain took %s, expected roughly the close timeout of %s", elapsed, tailer.closeTimeout)
	}
}
