// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package file

import (
	"fmt"
	"hash/crc64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
)

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

func (suite *FingerprintTestSuite) createTailer() *Tailer {
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
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
		FileOpener:      opener.NewFileOpener(),
	}

	tailer := NewTailer(tailerOptions)
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

	maxLines := 2
	maxBytes := 1024
	linesToSkip := 1

	config := types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
		Count:               maxLines,
		CountToSkip:         linesToSkip,
		MaxBytes:            maxBytes,
	}

	text := "first data linesecond data line"
	table := crc64.MakeTable(crc64.ISO)
	expectedChecksum := crc64.Checksum([]byte(text), table)

	tailer := suite.createTailer()
	fingerprinter := NewFingerprinter(config, opener.NewFileOpener())
	receivedChecksum, _ := fingerprinter.ComputeFingerprint(tailer.file)
	suite.Equal(expectedChecksum, receivedChecksum.Value)
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
	maxBytes := 1024
	linesToSkip := 0

	config := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
		Count:               maxLines,
		CountToSkip:         linesToSkip,
		MaxBytes:            maxBytes,
	}

	// Expected: line should be cut off and hashed up to maxBytes (1024)
	expectedText := make([]byte, 1024)
	for i := range expectedText {
		expectedText[i] = 'A'
	}

	table := crc64.MakeTable(crc64.ISO)
	expectedChecksum := crc64.Checksum(expectedText, table)

	tailer := suite.createTailer()
	tailer.osFile = osFile

	fingerprinter := NewFingerprinter(*config, opener.NewFileOpener())
	receivedChecksum, _ := fingerprinter.ComputeFingerprint(tailer.file)

	suite.Equal(expectedChecksum, receivedChecksum.Value)
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
	maxLines := 10
	maxBytes := 1024
	linesToSkip := 0

	config := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
		Count:               maxLines,
		CountToSkip:         linesToSkip,
		MaxBytes:            maxBytes,
	}

	//Should hash up to default maxBytes (1024) since we are falling back to the default byte based configuration
	expectedText := "line1: " + line1Content + "\n" + "line2: " + line2Content[:209]

	table := crc64.MakeTable(crc64.ISO)
	expectedChecksum := crc64.Checksum([]byte(expectedText), table)

	tailer := suite.createTailer()

	fingerprinter := NewFingerprinter(*config, opener.NewFileOpener())
	receivedChecksum, _ := fingerprinter.ComputeFingerprint(tailer.file)

	suite.Equal(expectedChecksum, receivedChecksum.Value)
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

	maxLines := 2
	maxBytes := 1024
	linesToSkip := 2

	config := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
		Count:               maxLines,
		CountToSkip:         linesToSkip,
		MaxBytes:            maxBytes,
	}
	// Expected: skip "skip1\n" and "skip2\n", then fingerprint "keep1\n" and "keep2\n"
	expectedText := "keep1keep2"

	table := crc64.MakeTable(crc64.ISO)
	expectedChecksum := crc64.Checksum([]byte(expectedText), table)

	tailer := suite.createTailer()

	fingerprinter := NewFingerprinter(*config, opener.NewFileOpener())
	receivedChecksum, _ := fingerprinter.ComputeFingerprint(tailer.file)

	suite.Equal(expectedChecksum, receivedChecksum.Value)
}

func (suite *FingerprintTestSuite) TestLineBased_EmptyFile() {
	// Don't write anything to the file, so it remains empty
	suite.testFile.Sync()

	maxLines := 5
	maxBytes := 1024
	linesToSkip := 0

	config := &types.FingerprintConfig{
		Count:               maxLines,
		CountToSkip:         linesToSkip,
		MaxBytes:            maxBytes,
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
	}

	// Expected: empty file should return nil since we don't have any data to hash
	tailer := suite.createTailer()

	fingerprinter := NewFingerprinter(*config, opener.NewFileOpener())
	receivedChecksum, _ := fingerprinter.ComputeFingerprint(tailer.file)
	suite.Equal(uint64(0), receivedChecksum.Value, "Empty file should return fingerprint with Value=0")
}

// We don't have enough data to hash with the maxLines we configured we have neither the appropriate number of lines or bytes
func (suite *FingerprintTestSuite) TestLineBased_InsufficientData() {
	// Write only 2 lines to the file
	data := "line1\nline2\n"
	_, err := suite.testFile.WriteString(data)
	suite.Nil(err)
	suite.testFile.Sync()

	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()

	maxLines := 5
	maxBytes := 1024
	linesToSkip := 0

	config := &types.FingerprintConfig{
		Count:               maxLines,
		CountToSkip:         linesToSkip,
		MaxBytes:            maxBytes,
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
	}

	// Expected: should return nil because we have fewer lines than maxLines (2 lines vs 5 maxLines)
	tailer := suite.createTailer()
	tailer.osFile = osFile

	fingerprinter := NewFingerprinter(*config, opener.NewFileOpener())
	receivedChecksum, _ := fingerprinter.ComputeFingerprint(tailer.file)
	suite.Equal(uint64(0), receivedChecksum.Value, "Should return fingerprint with Value=0 when insufficient lines")
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
	maxBytes := 50
	bytesToSkip := 34
	fingerprintStrategy := types.FingerprintStrategyByteChecksum

	config := &types.FingerprintConfig{
		Count:               maxBytes,
		CountToSkip:         bytesToSkip,
		MaxBytes:            maxBytes,
		FingerprintStrategy: fingerprintStrategy,
	}

	// Expected: skip first 34 bytes, then fingerprint next 50 bytes
	expectedText := "this is the actual data we want to fingerprint for"
	table := crc64.MakeTable(crc64.ISO)
	expectedChecksum := crc64.Checksum([]byte(expectedText), table)

	tailer := suite.createTailer()
	tailer.osFile = osFile
	fingerprinter := NewFingerprinter(*config, opener.NewFileOpener())
	receivedChecksum, _ := fingerprinter.ComputeFingerprint(tailer.file)
	suite.Equal(expectedChecksum, receivedChecksum.Value)
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
	maxBytes := 1000
	bytesToSkip := 34
	fingerprintStrategy := types.FingerprintStrategyByteChecksum

	config := &types.FingerprintConfig{
		Count:               maxBytes,
		CountToSkip:         bytesToSkip,
		MaxBytes:            maxBytes,
		FingerprintStrategy: fingerprintStrategy,
	}

	// Expected: skip first 34 bytes, but unable to fingerprint since less than 1000 we configured
	tailer := suite.createTailer()
	tailer.osFile = osFile

	fingerprinter := NewFingerprinter(*config, opener.NewFileOpener())
	receivedChecksum, _ := fingerprinter.ComputeFingerprint(tailer.file)
	suite.Equal(uint64(0), receivedChecksum.Value, "Insufficient data after skip should return fingerprint with Value=0")
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

	maxBytes := 30
	bytesToSkip := 0
	fingerprintStrategy := types.FingerprintStrategyByteChecksum

	config := &types.FingerprintConfig{
		Count:               maxBytes,
		CountToSkip:         bytesToSkip,
		MaxBytes:            maxBytes,
		FingerprintStrategy: fingerprintStrategy,
	}
	// Expected: fingerprint first 30 bytes
	expectedText := "this data should be fingerprin"

	table := crc64.MakeTable(crc64.ISO)
	expectedChecksum := crc64.Checksum([]byte(expectedText), table)

	tailer := suite.createTailer()
	tailer.osFile = osFile

	fingerprinter := NewFingerprinter(*config, opener.NewFileOpener())
	receivedChecksum, _ := fingerprinter.ComputeFingerprint(tailer.file)
	suite.Equal(expectedChecksum, receivedChecksum.Value)
}

// We don't have enough data to hash with the maxBytes we configured
func (suite *FingerprintTestSuite) TestByteBased_InsufficientData() {
	data := "short data"

	_, err := suite.testFile.WriteString(data)
	suite.Nil(err)
	suite.testFile.Sync()

	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()

	maxBytes := 100
	bytesToSkip := 0
	fingerprintStrategy := types.FingerprintStrategyByteChecksum

	config := &types.FingerprintConfig{
		Count:               maxBytes,
		CountToSkip:         bytesToSkip,
		MaxBytes:            maxBytes,
		FingerprintStrategy: fingerprintStrategy,
	}

	// Expected: should return nil because we have less data than maxBytes
	tailer := suite.createTailer()
	tailer.osFile = osFile

	fingerprinter := NewFingerprinter(*config, opener.NewFileOpener())
	receivedChecksum, _ := fingerprinter.ComputeFingerprint(tailer.file)
	suite.Equal(uint64(0), receivedChecksum.Value, "Insufficient data should return fingerprint with Value=0")
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

	config := &types.FingerprintConfig{
		Count:               maxLines,
		CountToSkip:         linesToSkip,
		MaxBytes:            maxBytes,
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
	}

	// Expected: skip first line, then fingerprint remaining lines
	expectedText := "line2line3"

	table := crc64.MakeTable(crc64.ISO)
	expectedChecksum := crc64.Checksum([]byte(expectedText), table)

	tailer := suite.createTailer()
	tailer.osFile = osFile

	fingerprinter := NewFingerprinter(*config, opener.NewFileOpener())
	receivedChecksum, _ := fingerprinter.ComputeFingerprint(tailer.file)
	suite.Equal(expectedChecksum, receivedChecksum.Value)
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

	maxBytes := 21
	bytesToSkip := 10
	fingerprintStrategy := types.FingerprintStrategyByteChecksum

	config := &types.FingerprintConfig{
		Count:               maxBytes,
		CountToSkip:         bytesToSkip,
		MaxBytes:            maxBytes,
		FingerprintStrategy: fingerprintStrategy,
	}

	expectedText := "st data for byte mode"
	table := crc64.MakeTable(crc64.ISO)
	expectedChecksum := crc64.Checksum([]byte(expectedText), table)

	tailer := suite.createTailer()
	tailer.osFile = osFile

	fingerprinter := NewFingerprinter(*config, opener.NewFileOpener())
	receivedChecksum, _ := fingerprinter.ComputeFingerprint(tailer.file)
	suite.Equal(expectedChecksum, receivedChecksum.Value)
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

	config := &types.FingerprintConfig{
		Count:               maxLines,
		CountToSkip:         linesToSkip,
		MaxBytes:            maxBytes,
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
	}

	// Expected: should fingerprint all lines
	expectedText := "line1line2line3"

	table := crc64.MakeTable(crc64.ISO)
	expectedChecksum := crc64.Checksum([]byte(expectedText), table)

	tailer := suite.createTailer()
	tailer.osFile = osFile

	fingerprinter := NewFingerprinter(*config, opener.NewFileOpener())
	receivedChecksum, _ := fingerprinter.ComputeFingerprint(tailer.file)
	suite.Equal(expectedChecksum, receivedChecksum.Value)
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

	config := &types.FingerprintConfig{
		Count:               maxLines,
		CountToSkip:         linesToSkip,
		MaxBytes:            maxBytes,
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
	}

	tailer := suite.createTailer()
	tailer.osFile = osFile

	// Compute fingerprint (now returns uint64 directly)
	fingerprinter := NewFingerprinter(*config, opener.NewFileOpener())
	fingerprint, _ := fingerprinter.ComputeFingerprint(tailer.file)

	expectedText := "line 1: important data" + "line 2: more important data"
	table := crc64.MakeTable(crc64.ISO)
	expectedFingerprint := crc64.Checksum([]byte(expectedText), table)

	// Verify it's not zero (meaning it was computed successfully)
	suite.Equal(expectedFingerprint, fingerprint.Value)
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

	maxBytes := 20
	bytesToSkip := 14
	fingerprintStrategy := types.FingerprintStrategyByteChecksum

	config := &types.FingerprintConfig{
		Count:               maxBytes,
		CountToSkip:         bytesToSkip,
		MaxBytes:            maxBytes,
		FingerprintStrategy: fingerprintStrategy,
	}

	tailer := suite.createTailer()
	tailer.osFile = osFile
	fingerprinter := NewFingerprinter(*config, opener.NewFileOpener())
	fingerprint, _ := fingerprinter.ComputeFingerprint(tailer.file)

	textToHash := "thisisexactly20chars"
	table := crc64.MakeTable(crc64.ISO)
	expectedHash := crc64.Checksum([]byte(textToHash), table)

	suite.Equal(expectedHash, fingerprint.Value)
}

func (suite *FingerprintTestSuite) TestEmptyFile_And_SkippingMoreThanFileSize() {
	// Test 1: Empty file
	maxLines := 5
	maxBytes := 1024
	bytesToSkip := 0
	fingerprintStrategy := types.FingerprintStrategyByteChecksum
	config := &types.FingerprintConfig{
		Count:               maxLines,
		CountToSkip:         bytesToSkip,
		MaxBytes:            maxBytes,
		FingerprintStrategy: fingerprintStrategy,
	}

	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)

	tailer := suite.createTailer()
	tailer.osFile = osFile

	fingerprinter := NewFingerprinter(*config, opener.NewFileOpener())
	fingerprint, _ := fingerprinter.ComputeFingerprint(tailer.file)

	suite.Equal(uint64(0), fingerprint.Value, "Empty file should return fingerprint with Value=0")
	osFile.Close()

	// Test 2: Insufficient data after skipping
	_, err = suite.testFile.WriteString("short")
	suite.Nil(err)
	suite.testFile.Sync()

	bytesToSkip = 10 // More than file size
	config.CountToSkip = bytesToSkip

	osFile, err = os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()

	tailer = suite.createTailer()
	tailer.osFile = osFile

	fingerprinter = NewFingerprinter(*config, opener.NewFileOpener())
	fingerprint, _ = fingerprinter.ComputeFingerprint(tailer.file)

	suite.Equal(uint64(0), fingerprint.Value, "Insufficient data should return fingerprint with Value=0")
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

	config := &types.FingerprintConfig{
		Count:               maxLines,
		CountToSkip:         linesToSkip,
		MaxBytes:            maxBytes,
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
	}

	tailer := suite.createTailer()
	tailer.osFile = osFile

	fingerprinter := NewFingerprinter(*config, opener.NewFileOpener())
	fingerprint, _ := fingerprinter.ComputeFingerprint(tailer.file)

	expectedText := strings.Repeat("X", 80)
	table := crc64.MakeTable(crc64.ISO)
	expectedHash := crc64.Checksum([]byte(expectedText), table)
	// Should still compute a fingerprint even with truncation
	suite.Equal(expectedHash, fingerprint.Value)
}

// Tests the "whichever comes first" logic (X lines or Y bytes)
func (suite *FingerprintTestSuite) TestXLinesOrYBytesFirstHash() {
	lines := []string{
		strings.Repeat("A", 268) + "\n",
		strings.Repeat("B", 268) + "\n",
		strings.Repeat("C", 268) + "\n",
		strings.Repeat("D", 268) + "\n",
	}

	for _, line := range lines {
		_, err := suite.testFile.WriteString(line)
		suite.Nil(err)
	}
	suite.testFile.Sync()

	maxLines := 5
	maxBytes := 1024
	linesToSkip := 0

	config := &types.FingerprintConfig{
		Count:               maxLines,
		CountToSkip:         linesToSkip,
		MaxBytes:            maxBytes,
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
	}

	tailer := suite.createTailer()

	fingerprinter := NewFingerprinter(*config, opener.NewFileOpener())
	fingerprint, _ := fingerprinter.ComputeFingerprint(tailer.file)

	fmt.Println(lines)
	stringToHash := strings.Repeat("A", 268) + "\n" + strings.Repeat("B", 268) + "\n" + strings.Repeat("C", 268) + "\n" + strings.Repeat("D", 217)
	table := crc64.MakeTable(crc64.ISO)
	expectedHash := crc64.Checksum([]byte(stringToHash), table)

	suite.Equal(expectedHash, fingerprint.Value)
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

	fpConfig := &types.FingerprintConfig{
		Count:               maxLines,
		CountToSkip:         linesToSkip,
		MaxBytes:            maxBytes,
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
	}

	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)

	tailer := suite.createTailer()
	tailer.osFile = osFile

	fingerprinter := NewFingerprinter(*fpConfig, opener.NewFileOpener())
	fingerprint1, _ := fingerprinter.ComputeFingerprint(tailer.file)

	osFile.Close()

	// Reset file for next test
	suite.testFile.Seek(0, 0)
	suite.testFile.Truncate(0)
	_, err = suite.testFile.WriteString(data)
	suite.Nil(err)
	suite.testFile.Sync()

	textToHash1 := "line2line3"
	table := crc64.MakeTable(crc64.ISO)
	expectedHash1 := crc64.Checksum([]byte(textToHash1), table)
	suite.Equal(expectedHash1, fingerprint1.Value)

	maxLines = 2
	maxBytes = 10
	linesToSkip = 0

	fpConfig = &types.FingerprintConfig{
		Count:               maxLines,
		CountToSkip:         linesToSkip,
		MaxBytes:            maxBytes,
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
	}

	osFile, err = os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()

	tailer = suite.createTailer()
	tailer.osFile = osFile

	fingerprinter = NewFingerprinter(*fpConfig, opener.NewFileOpener())
	fingerprint2, _ := fingerprinter.ComputeFingerprint(tailer.file)

	textToHash2 := "line1line"
	table = crc64.MakeTable(crc64.ISO)
	expectedHash2 := crc64.Checksum([]byte(textToHash2), table)
	suite.Equal(expectedHash2, fingerprint2.Value)
}

func (suite *FingerprintTestSuite) TestLineBased_SkipAndMaxMidLine() {
	// The content to hash is a single line, and this line is longer than maxBytes.
	// The tailer should skip the specified number of lines, read the next line, truncate it to maxBytes, and hash that.

	lines := []string{
		"this line should be skipped\n",
		"ok we're trying something new here with a long, long, long, long, line.\n",
	}

	for _, line := range lines {
		_, err := suite.testFile.WriteString(line)
		suite.Nil(err)
	}
	suite.testFile.Sync()

	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()

	maxLines := 1
	maxBytes := 26
	linesToSkip := 1

	config := &types.FingerprintConfig{
		Count:               maxLines,
		CountToSkip:         linesToSkip,
		MaxBytes:            maxBytes,
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
	}

	// Expected: the new implementation returns nil when there's insufficient data after skipping

	tailer := suite.createTailer()

	fingerprinter := NewFingerprinter(*config, opener.NewFileOpener())
	receivedChecksum, _ := fingerprinter.ComputeFingerprint(tailer.file)

	suite.Equal(uint64(0), receivedChecksum.Value, "Should return fingerprint with Value=0 when there's insufficient data after skipping")
}

// Tests whether or not rotation was accurately detected
func (suite *FingerprintTestSuite) TestDidRotateViaFingerprint() {
	// 1. Start with a file with content and create a tailer.
	suite.T().Log("Writing initial content and creating tailer")
	_, err := suite.testFile.WriteString("line 1\nline 2\nline 3\n")
	suite.Nil(err)
	suite.Nil(suite.testFile.Sync())

	config := &types.FingerprintConfig{
		Count:               1,
		CountToSkip:         0,
		MaxBytes:            102400,
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
	}
	tailer := suite.createTailer()
	fingerprinter := NewFingerprinter(*config, opener.NewFileOpener())

	// Initialize osFile and fullpath for DidRotate() filesystem checks
	osFile, err := os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()
	tailer.osFile = osFile
	tailer.fullpath = suite.testPath

	// Compute initial fingerprint
	initialFingerprint, _ := fingerprinter.ComputeFingerprint(tailer.file)
	suite.NotNil(initialFingerprint)
	suite.True(initialFingerprint.ValidFingerprint())

	table := crc64.MakeTable(crc64.ISO)
	expectedChecksum := crc64.Checksum([]byte("line 1"), table)
	suite.Equal(expectedChecksum, initialFingerprint.Value)

	// Set the fingerprint on the tailer so DidRotateViaFingerprint can use it
	tailer.fingerprint = initialFingerprint

	// 2. Immediately check for rotation. It should be false as the file is unchanged.
	suite.T().Log("Checking for rotation on unchanged file")
	rotated, err := tailer.DidRotateViaFingerprint(fingerprinter)
	suite.Nil(err)
	suite.False(rotated, "Should not detect rotation on an unchanged file")

	// 3. Truncate the file, which simulates a rotation.
	// valid -> invalid triggers filesystem check.
	// filesystem check returns false (no rotation detected).
	suite.T().Log("Truncating file to simulate rotation")
	suite.Nil(suite.testFile.Truncate(0))
	_, err = suite.testFile.Seek(0, 0)
	suite.Nil(err)
	suite.Nil(suite.testFile.Sync())
	rotated, err = tailer.DidRotateViaFingerprint(fingerprinter)
	suite.Nil(err)
	suite.False(rotated, "Truncation not detected via filesystem check when lastReadOffset=0")

	// 4. Simulate a full file replacement (e.g. logrotate with 'create' directive).
	suite.T().Log("Simulating file replacement with different content")
	_, err = suite.testFile.WriteString("a completely new file\n")
	suite.Nil(err)
	suite.Nil(suite.testFile.Sync())

	// We 're-arm' the tailer, as if the launcher had picked up the new file.
	// This tailer now considers the current content ("a completely new file") as its baseline.
	tailer = suite.createTailer()

	// Re-open the file and set up the tailer for filesystem checks
	osFile, err = os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile.Close()
	tailer.osFile = osFile
	tailer.fullpath = suite.testPath

	fingerprinter = NewFingerprinter(*config, opener.NewFileOpener())
	newFingerprint, _ := fingerprinter.ComputeFingerprint(tailer.file)
	suite.NotNil(newFingerprint)
	suite.True(newFingerprint.ValidFingerprint())

	expectedChecksum = crc64.Checksum([]byte("a completely new file"), table)
	suite.Equal(expectedChecksum, newFingerprint.Value)

	// Set the fingerprint on the new tailer
	tailer.fingerprint = newFingerprint

	// Check for rotation immediately after re-arming. Since the file hasn't changed
	// since the tailer was created, it should report no rotation. Its internal fingerprint
	// matches the file's current fingerprint.
	rotated, err = tailer.DidRotateViaFingerprint(fingerprinter)
	suite.Nil(err)
	suite.False(rotated, "Should not detect rotation immediately after creating a new tailer on a file")

	expectedChecksum = crc64.Checksum([]byte("a completely new file"), table)
	suite.Equal(expectedChecksum, newFingerprint.Value)

	// Now, modify the file again. This change *should* be detected as a rotation.
	suite.T().Log("Simulating another rotation on the new file")
	suite.Nil(suite.testFile.Truncate(0))
	_, err = suite.testFile.Seek(0, 0)
	suite.Nil(err)
	_, err = suite.testFile.WriteString("even more different content\n")
	suite.Nil(err)
	suite.Nil(suite.testFile.Sync())

	rotated, err = tailer.DidRotateViaFingerprint(fingerprinter)
	suite.Nil(err)
	suite.True(rotated, "Should detect rotation after file content changes")
	expectedChecksum = crc64.Checksum([]byte("even more different content"), table)
	receivedChecksum, _ := fingerprinter.ComputeFingerprint(tailer.file)
	suite.Equal(expectedChecksum, receivedChecksum.Value)

	// 5. Test case with an empty file where both fingerprints are invalid.
	// When both old and new fingerprints are invalid, we fall back to filesystem checks.
	suite.T().Log("Testing rotation detection with both fingerprints invalid")
	suite.Nil(suite.testFile.Truncate(0))
	_, err = suite.testFile.Seek(0, 0)
	suite.Nil(err)
	suite.Nil(suite.testFile.Sync())
	tailer = suite.createTailer()

	// Open the file so the tailer has a valid osFile handle for DidRotate() filesystem checks
	osFile3, err := os.Open(suite.testPath)
	suite.Nil(err)
	defer osFile3.Close()
	tailer.osFile = osFile3
	tailer.fullpath = suite.testPath

	fingerprinter = NewFingerprinter(*config, opener.NewFileOpener())
	emptyFingerprint, _ := fingerprinter.ComputeFingerprint(tailer.file)
	suite.Equal(uint64(0), emptyFingerprint.Value, "Fingerprint of an empty file should have Value=0")

	// Set the fingerprint on the tailer (invalid)
	tailer.fingerprint = emptyFingerprint

	// With both fingerprints invalid (Value=0), DidRotateViaFingerprint falls back to filesystem checks.
	// Since we just opened the same file and nothing has changed, filesystem checks should return false.
	rotated, err = tailer.DidRotateViaFingerprint(fingerprinter)
	suite.Nil(err)
	suite.False(rotated, "Should not detect rotation when both fingerprints are invalid and filesystem shows no change")

	// 6. Test case with invalid -> valid transition (empty file gets content)
	// old invalid + new valid => rotation detected
	suite.T().Log("Testing rotation detection from invalid to valid fingerprint")
	suite.Nil(suite.testFile.Truncate(0))
	suite.Nil(suite.testFile.Sync())
	tailer = suite.createTailer()

	// Set baseline as invalid (empty file)
	invalidFingerprint, _ := fingerprinter.ComputeFingerprint(tailer.file)
	suite.Equal(uint64(0), invalidFingerprint.Value)
	tailer.fingerprint = invalidFingerprint

	// Now add content to the file
	_, err = suite.testFile.WriteString("new content after empty\n")
	suite.Nil(err)
	suite.Nil(suite.testFile.Sync())

	// Should detect rotation: invalid -> valid (non-zero) without needing filesystem checks
	rotated, err = tailer.DidRotateViaFingerprint(fingerprinter)
	suite.Nil(err)
	suite.True(rotated, "Should detect rotation when transitioning from invalid to valid fingerprint")
}

func (suite *FingerprintTestSuite) TestLineBased_FallbackToByteBased() {
	// Write only 2 lines to the file
	data := "line1\nline2\n"
	_, err := suite.testFile.WriteString(data)
	suite.Nil(err)
	suite.testFile.Sync()

	// Try to skip 3 lines (more than available) with small maxBytes to trigger LimitedReader exhaustion
	maxLines := 2
	maxBytes := 12   // Small enough to trigger LimitedReader exhaustion during skip
	linesToSkip := 3 // More lines than available (only 2 lines in file)

	config := &types.FingerprintConfig{
		Count:               maxLines,
		CountToSkip:         linesToSkip,
		MaxBytes:            maxBytes,
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
	}

	tailer := suite.createTailer()
	fingerprinter := NewFingerprinter(*config, opener.NewFileOpener())
	fingerprint, _ := fingerprinter.ComputeFingerprint(tailer.file)

	// Since we're trying to skip more lines than exist, and the LimitedReader exhausts,
	// this should trigger the fallback to byte-based fingerprinting
	// The fallback should read from the beginning of the file (after any byte skip)

	// Expected: the new implementation returns fingerprint with Value=0 when there's insufficient data
	suite.Equal(uint64(0), fingerprint.Value, "Should return fingerprint with Value=0 when there's insufficient data for fingerprinting")
}

func (suite *FingerprintTestSuite) TestFingerprintConfigFallback() {
	// tests the fallback logic between file-specific and global configs
	testData := "line1\nline2\nline3\nline4\n"
	_, err := suite.testFile.WriteString(testData)
	suite.Nil(err)
	suite.testFile.Sync()

	testCases := []struct {
		name                      string
		globalConfig              types.FingerprintConfig
		fileConfig                *types.FingerprintConfig
		expectedShouldFingerprint bool
		expectedStrategy          types.FingerprintStrategy
		expectedCount             int
		expectedCountToSkip       int
		expectedMaxBytes          int
	}{
		{
			name: "file_config_with_strategy_overrides_global",
			globalConfig: types.FingerprintConfig{
				FingerprintStrategy: types.FingerprintStrategyDisabled,
				Count:               1,
				CountToSkip:         0,
				MaxBytes:            1000,
			},
			fileConfig: &types.FingerprintConfig{
				FingerprintStrategy: types.FingerprintStrategyLineChecksum,
				Count:               2,
				CountToSkip:         1,
				MaxBytes:            2000,
			},
			expectedShouldFingerprint: true,
			expectedStrategy:          types.FingerprintStrategyLineChecksum,
			expectedCount:             2,
			expectedCountToSkip:       1,
			expectedMaxBytes:          2000,
		},
		{
			name: "file_config_disabled_overrides_global_enabled",
			globalConfig: types.FingerprintConfig{
				FingerprintStrategy: types.FingerprintStrategyLineChecksum,
				Count:               1,
				CountToSkip:         0,
				MaxBytes:            1000,
			},
			fileConfig: &types.FingerprintConfig{
				FingerprintStrategy: types.FingerprintStrategyDisabled,
				Count:               1,
				CountToSkip:         0,
				MaxBytes:            1000,
			},
			expectedShouldFingerprint: false,
			expectedStrategy:          types.FingerprintStrategyDisabled,
		},
		{
			name: "file_config_empty_strategy_falls_back_to_global",
			globalConfig: types.FingerprintConfig{
				FingerprintStrategy: types.FingerprintStrategyByteChecksum,
				Count:               512,
				CountToSkip:         0,
				MaxBytes:            0,
			},
			fileConfig: &types.FingerprintConfig{
				FingerprintStrategy: "", // Empty strategy should fall back to global
				Count:               2,
				CountToSkip:         1,
				MaxBytes:            2000,
			},
			expectedShouldFingerprint: true,
			expectedStrategy:          types.FingerprintStrategyByteChecksum,
			expectedCount:             512,
			expectedCountToSkip:       0,
			expectedMaxBytes:          0,
		},
		{
			name: "no_file_config_falls_back_to_global",
			globalConfig: types.FingerprintConfig{
				FingerprintStrategy: types.FingerprintStrategyLineChecksum,
				Count:               3,
				CountToSkip:         0,
				MaxBytes:            1500,
			},
			fileConfig:                nil, // No file config should fall back to global
			expectedShouldFingerprint: true,
			expectedStrategy:          types.FingerprintStrategyLineChecksum,
			expectedCount:             3,
			expectedCountToSkip:       0,
			expectedMaxBytes:          1500,
		},
		{
			name: "file_config_nil_strategy_falls_back_to_global",
			globalConfig: types.FingerprintConfig{
				FingerprintStrategy: types.FingerprintStrategyLineChecksum,
				Count:               1,
				CountToSkip:         0,
				MaxBytes:            1000,
			},
			fileConfig: &types.FingerprintConfig{
				// FingerprintStrategy not set
				Count:       5,
				CountToSkip: 2,
				MaxBytes:    3000,
			},
			expectedShouldFingerprint: true,
			expectedStrategy:          types.FingerprintStrategyLineChecksum,
			expectedCount:             1, // Should use global config values
			expectedCountToSkip:       0,
			expectedMaxBytes:          1000,
		},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(_ *testing.T) {
			var source *sources.ReplaceableSource
			if tc.fileConfig != nil {
				source = sources.NewReplaceableSource(sources.NewLogSource("", &config.LogsConfig{
					Type:              config.FileType,
					Path:              suite.testPath,
					FingerprintConfig: tc.fileConfig,
				}))
			} else {
				source = sources.NewReplaceableSource(sources.NewLogSource("", &config.LogsConfig{
					Type: config.FileType,
					Path: suite.testPath,
				}))
			}

			// Create fingerprinter with global config
			fingerprinter := NewFingerprinter(tc.globalConfig, opener.NewFileOpener())

			file := NewFile(suite.testPath, source.UnderlyingSource(), false)

			shouldFingerprint := fingerprinter.ShouldFileFingerprint(file)
			suite.Equal(tc.expectedShouldFingerprint, shouldFingerprint,
				"ShouldFileFingerprint should return %v for test case %s", tc.expectedShouldFingerprint, tc.name)

			fingerprint, err := fingerprinter.ComputeFingerprint(file)
			suite.Nil(err, "ComputeFingerprint should not return error for test case %s", tc.name)

			if tc.expectedShouldFingerprint {
				// If fingerprinting is enabled, verify the config used
				suite.NotNil(fingerprint.Config, "Fingerprint config should not be nil for test case %s", tc.name)
				suite.Equal(tc.expectedStrategy, fingerprint.Config.FingerprintStrategy,
					"Fingerprint strategy should be %s for test case %s", tc.expectedStrategy, tc.name)
				suite.Equal(tc.expectedCount, fingerprint.Config.Count,
					"Fingerprint count should be %d for test case %s", tc.expectedCount, tc.name)
				suite.Equal(tc.expectedCountToSkip, fingerprint.Config.CountToSkip,
					"Fingerprint countToSkip should be %d for test case %s", tc.expectedCountToSkip, tc.name)
				suite.Equal(tc.expectedMaxBytes, fingerprint.Config.MaxBytes,
					"Fingerprint maxBytes should be %d for test case %s", tc.expectedMaxBytes, tc.name)
			} else {
				// If fingerprinting is disabled, return invalid fingerprint
				suite.Equal(uint64(types.InvalidFingerprintValue), fingerprint.Value,
					"Fingerprint value should be invalid for disabled test case %s", tc.name)
			}
		})
	}
}

func (suite *FingerprintTestSuite) TestFingerprintConfigPrecedence() {
	// check file-specific configs take precedence over global configs
	testData := "line1\nline2\nline3\nline4\n"
	_, err := suite.testFile.WriteString(testData)
	suite.Nil(err)
	suite.testFile.Sync()

	// global config == line_checksum
	globalConfig := types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
		Count:               1,
		CountToSkip:         0,
		MaxBytes:            1000,
	}

	// File config == byte_checksum - should override global
	fileConfig := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyByteChecksum,
		Count:               512,
		CountToSkip:         0,
		MaxBytes:            0,
	}

	source := sources.NewReplaceableSource(sources.NewLogSource("", &config.LogsConfig{
		Type:              config.FileType,
		Path:              suite.testPath,
		FingerprintConfig: fileConfig,
	}))

	fingerprinter := NewFingerprinter(globalConfig, opener.NewFileOpener())

	file := NewFile(suite.testPath, source.UnderlyingSource(), false)

	// Should use file config (byte_checksum), not global config (line_checksum)
	shouldFingerprint := fingerprinter.ShouldFileFingerprint(file)
	suite.True(shouldFingerprint, "Should fingerprint with file config")

	fingerprint, err := fingerprinter.ComputeFingerprint(file)
	suite.Nil(err, "ComputeFingerprint should not return error")
	suite.NotNil(fingerprint.Config, "Fingerprint config should not be nil")
	suite.Equal(types.FingerprintStrategyByteChecksum, fingerprint.Config.FingerprintStrategy,
		"Should use file config strategy (byte_checksum), not global config (line_checksum)")
	suite.Equal(512, fingerprint.Config.Count,
		"Should use file config count (512), not global config count (1)")
}

func (suite *FingerprintTestSuite) TestFingerprintConfigEdgeCases() {
	// Write test data
	testData := "line1\nline2\nline3\nline4\n"
	_, err := suite.testFile.WriteString(testData)
	suite.Nil(err)
	suite.testFile.Sync()

	testCases := []struct {
		name                      string
		globalConfig              types.FingerprintConfig
		fileConfig                *types.FingerprintConfig
		expectedShouldFingerprint bool
		description               string
	}{
		{
			name: "file_config_with_zero_values",
			globalConfig: types.FingerprintConfig{
				FingerprintStrategy: types.FingerprintStrategyLineChecksum,
				Count:               1,
				CountToSkip:         0,
				MaxBytes:            1000,
			},
			fileConfig: &types.FingerprintConfig{
				FingerprintStrategy: types.FingerprintStrategyLineChecksum,
				Count:               0, // Zero count
				CountToSkip:         0,
				MaxBytes:            0, // Zero maxBytes
			},
			expectedShouldFingerprint: true,
			description:               "File config with zero values should still be used",
		},
		{
			name: "file_config_with_negative_values",
			globalConfig: types.FingerprintConfig{
				FingerprintStrategy: types.FingerprintStrategyLineChecksum,
				Count:               1,
				CountToSkip:         0,
				MaxBytes:            1000,
			},
			fileConfig: &types.FingerprintConfig{
				FingerprintStrategy: types.FingerprintStrategyLineChecksum,
				Count:               -1, // Negative count
				CountToSkip:         -1, // Negative countToSkip
				MaxBytes:            -1, // Negative maxBytes
			},
			expectedShouldFingerprint: true,
			description:               "File config with negative values should still be used",
		},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(_ *testing.T) {
			// Create source with the file config
			source := sources.NewReplaceableSource(sources.NewLogSource("", &config.LogsConfig{
				Type:              config.FileType,
				Path:              suite.testPath,
				FingerprintConfig: tc.fileConfig,
			}))

			// Create fingerprinter with global config
			fingerprinter := NewFingerprinter(tc.globalConfig, opener.NewFileOpener())

			// Create file object
			file := NewFile(suite.testPath, source.UnderlyingSource(), false)

			// Test ShouldFileFingerprint
			shouldFingerprint := fingerprinter.ShouldFileFingerprint(file)
			suite.Equal(tc.expectedShouldFingerprint, shouldFingerprint,
				"ShouldFileFingerprint should return %v for %s: %s",
				tc.expectedShouldFingerprint, tc.name, tc.description)

			// Test ComputeFingerprint
			fingerprint, err := fingerprinter.ComputeFingerprint(file)
			suite.Nil(err, "ComputeFingerprint should not return error for %s: %s", tc.name, tc.description)

			if tc.expectedShouldFingerprint {
				suite.NotNil(fingerprint.Config, "Fingerprint config should not be nil for %s", tc.name)
				// Verify that file config values are used (even if they're zero or negative)
				suite.Equal(tc.fileConfig.Count, fingerprint.Config.Count,
					"Should use file config count for %s", tc.name)
				suite.Equal(tc.fileConfig.CountToSkip, fingerprint.Config.CountToSkip,
					"Should use file config countToSkip for %s", tc.name)
				suite.Equal(tc.fileConfig.MaxBytes, fingerprint.Config.MaxBytes,
					"Should use file config maxBytes for %s", tc.name)
			}
		})
	}
}

// TestFingerprintConfigInfo tests the FingerprintConfigInfo struct and its Info() method
func TestFingerprintConfigInfo(t *testing.T) {
	testCases := []struct {
		name           string
		config         *types.FingerprintConfig
		expectedOutput []string
	}{
		{
			name: "per_source_line_checksum_with_maxbytes",
			config: &types.FingerprintConfig{
				FingerprintStrategy: types.FingerprintStrategyLineChecksum,
				Count:               10,
				CountToSkip:         5,
				MaxBytes:            1024,
				Source:              types.FingerprintConfigSourcePerSource,
			},
			expectedOutput: []string{
				"Source: per-source",
				"Strategy: line_checksum",
				"Count: 10",
				"CountToSkip: 5",
				"MaxBytes: 1024",
			},
		},
		{
			name: "per_source_byte_checksum_no_maxbytes",
			config: &types.FingerprintConfig{
				FingerprintStrategy: types.FingerprintStrategyByteChecksum,
				Count:               512,
				CountToSkip:         0,
				MaxBytes:            0,
				Source:              types.FingerprintConfigSourcePerSource,
			},
			expectedOutput: []string{
				"Source: per-source",
				"Strategy: byte_checksum",
				"Count: 512",
				"CountToSkip: 0",
			},
		},
		{
			name: "global_line_checksum_with_maxbytes",
			config: &types.FingerprintConfig{
				FingerprintStrategy: types.FingerprintStrategyLineChecksum,
				Count:               1,
				CountToSkip:         0,
				MaxBytes:            10000,
				Source:              types.FingerprintConfigSourceGlobal,
			},
			expectedOutput: []string{
				"Source: global",
				"Strategy: line_checksum",
				"Count: 1",
				"CountToSkip: 0",
				"MaxBytes: 10000",
			},
		},
		{
			name: "global_byte_checksum",
			config: &types.FingerprintConfig{
				FingerprintStrategy: types.FingerprintStrategyByteChecksum,
				Count:               2048,
				CountToSkip:         100,
				MaxBytes:            0,
				Source:              types.FingerprintConfigSourceGlobal,
			},
			expectedOutput: []string{
				"Source: global",
				"Strategy: byte_checksum",
				"Count: 2048",
				"CountToSkip: 100",
			},
		},
		{
			name: "disabled_strategy_per_source",
			config: &types.FingerprintConfig{
				FingerprintStrategy: types.FingerprintStrategyDisabled,
				Count:               0,
				CountToSkip:         0,
				MaxBytes:            0,
				Source:              types.FingerprintConfigSourcePerSource,
			},
			expectedOutput: []string{
				"Source: per-source",
				"Strategy: disabled",
			},
		},
		{
			name: "disabled_strategy_global",
			config: &types.FingerprintConfig{
				FingerprintStrategy: types.FingerprintStrategyDisabled,
				Source:              types.FingerprintConfigSourceGlobal,
			},
			expectedOutput: []string{
				"Source: global",
				"Strategy: disabled",
			},
		},
		{
			name: "line_checksum_with_zero_values",
			config: &types.FingerprintConfig{
				FingerprintStrategy: types.FingerprintStrategyLineChecksum,
				Count:               0,
				CountToSkip:         0,
				MaxBytes:            0,
				Source:              types.FingerprintConfigSourcePerSource,
			},
			expectedOutput: []string{
				"Source: per-source",
				"Strategy: line_checksum",
				"Count: 0",
				"CountToSkip: 0",
				"MaxBytes: 0",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			info := NewFingerprintConfigInfo(tc.config)

			// Test InfoKey
			if info.InfoKey() != "Fingerprint Config" {
				t.Errorf("Expected InfoKey to be 'Fingerprint Config', got '%s'", info.InfoKey())
			}

			// Test Info output
			output := info.Info()
			if len(output) != len(tc.expectedOutput) {
				t.Fatalf("Expected %d output lines, got %d.\nExpected: %v\nGot: %v",
					len(tc.expectedOutput), len(output), tc.expectedOutput, output)
			}

			for i, expected := range tc.expectedOutput {
				if output[i] != expected {
					t.Errorf("Line %d: expected '%s', got '%s'", i, expected, output[i])
				}
			}
		})
	}
}

// TestComputeFingerprintPreservesConfigWhenDisabled tests that disabled configs are preserved in fingerprint
func (suite *FingerprintTestSuite) TestComputeFingerprintPreservesConfigWhenDisabled() {
	globalConfig := types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyByteChecksum,
		Count:               1024,
	}
	fingerprinter := NewFingerprinter(globalConfig, opener.NewFileOpener())

	// Write test data
	_, err := suite.testFile.WriteString("test data for fingerprinting\n")
	suite.Nil(err)
	suite.testFile.Sync()

	// Test with disabled per-source config
	disabledConfig := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyDisabled,
		Count:               500,
	}
	sourceConfig := &config.LogsConfig{
		Type:              config.FileType,
		Path:              suite.testPath,
		FingerprintConfig: disabledConfig,
	}
	source := sources.NewLogSource("test", sourceConfig)
	file := &File{
		Path:   suite.testPath,
		Source: sources.NewReplaceableSource(source),
	}

	fingerprint, err := fingerprinter.ComputeFingerprint(file)
	suite.Nil(err)
	suite.NotNil(fingerprint)
	suite.Equal(types.InvalidFingerprintValue, int(fingerprint.Value), "Fingerprint value should be invalid when disabled")
	suite.NotNil(fingerprint.Config, "Config should be preserved even when disabled")
	suite.Equal(types.FingerprintStrategyDisabled, fingerprint.Config.FingerprintStrategy)
	suite.Equal(types.FingerprintConfigSourcePerSource, fingerprint.Config.Source, "Config should show it was disabled at per-source level")
	suite.Equal(500, fingerprint.Config.Count, "Config values should be preserved")
}

// TestComputeFingerprintWithEnabledConfig tests fingerprinting with enabled config includes Source field
func (suite *FingerprintTestSuite) TestComputeFingerprintWithEnabledConfig() {
	globalConfig := types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyByteChecksum,
		Count:               1024,
		Source:              types.FingerprintConfigSourceGlobal,
	}
	fingerprinter := NewFingerprinter(globalConfig, opener.NewFileOpener())

	// Write test data
	testData := "test data for fingerprinting\n"
	_, err := suite.testFile.WriteString(testData)
	suite.Nil(err)
	suite.testFile.Sync()

	// Test with per-source config
	perSourceConfig := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyByteChecksum,
		Count:               100,
		CountToSkip:         0,
		Source:              types.FingerprintConfigSourcePerSource,
	}
	sourceConfig := &config.LogsConfig{
		Type:              config.FileType,
		Path:              suite.testPath,
		FingerprintConfig: perSourceConfig,
	}
	source := sources.NewLogSource("test", sourceConfig)
	file := &File{
		Path:   suite.testPath,
		Source: sources.NewReplaceableSource(source),
	}

	fingerprint, err := fingerprinter.ComputeFingerprint(file)
	suite.Nil(err)
	suite.NotNil(fingerprint)
	suite.NotEqual(types.InvalidFingerprintValue, fingerprint.Value, "Fingerprint should have valid value")
	suite.NotNil(fingerprint.Config)
	suite.Equal(types.FingerprintStrategyByteChecksum, fingerprint.Config.FingerprintStrategy)
	suite.Equal(types.FingerprintConfigSourcePerSource, fingerprint.Config.Source, "Per-source config should have Source='per-source'")

	// Test with global config (no per-source config)
	sourceConfig2 := &config.LogsConfig{
		Type: config.FileType,
		Path: suite.testPath,
	}
	source2 := sources.NewLogSource("test2", sourceConfig2)
	file2 := &File{
		Path:   suite.testPath,
		Source: sources.NewReplaceableSource(source2),
	}

	fingerprint2, err2 := fingerprinter.ComputeFingerprint(file2)
	suite.Nil(err2)
	suite.NotNil(fingerprint2)
	suite.NotEqual(types.InvalidFingerprintValue, fingerprint2.Value)
	suite.NotNil(fingerprint2.Config)
	suite.Equal(types.FingerprintStrategyByteChecksum, fingerprint2.Config.FingerprintStrategy)
	suite.Equal(types.FingerprintConfigSourceGlobal, fingerprint2.Config.Source, "Global config should have Source='global'")
}

// TestDefaultConfigsHaveSource tests that default fallback configs have Source set
func TestDefaultConfigsHaveSource(t *testing.T) {
	if defaultBytesConfig.Source != types.FingerprintConfigSourceDefault {
		t.Errorf("defaultBytesConfig should have Source='default', got '%s'", defaultBytesConfig.Source)
	}

	if defaultLinesConfig.Source != types.FingerprintConfigSourceDefault {
		t.Errorf("defaultLinesConfig should have Source='default', got '%s'", defaultLinesConfig.Source)
	}
}
