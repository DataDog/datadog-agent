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
	fingerprinter := NewFingerprinter(true, config)
	receivedChecksum := fingerprinter.ComputeFingerprint(tailer.file)
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
	maxBytes := 2048
	linesToSkip := 0

	config := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
		Count:               maxLines,
		CountToSkip:         linesToSkip,
		MaxBytes:            maxBytes,
	}

	// Expected: line should be cut off and hashed up to maxBytes (2048)
	expectedText := make([]byte, 2048)
	for i := range expectedText {
		expectedText[i] = 'A'
	}

	table := crc64.MakeTable(crc64.ISO)
	expectedChecksum := crc64.Checksum(expectedText, table)

	tailer := suite.createTailer()
	tailer.osFile = osFile

	fingerprinter := NewFingerprinter(true, *config)
	receivedChecksum := fingerprinter.ComputeFingerprint(tailer.file)

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
	maxBytes := 1500
	linesToSkip := 0

	config := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
		Count:               maxLines,
		CountToSkip:         linesToSkip,
		MaxBytes:            maxBytes,
	}

	//Should hash up to maxBytes (1500) which includes the 807 bytes of line1 and the first 693 bytes of line2 (which accounts for the text "line1: ", "line2: ", and the line break)
	expectedText := "line1: " + line1Content + "line2: " + line2Content[:685]

	table := crc64.MakeTable(crc64.ISO)
	fmt.Println(len(expectedText))
	expectedChecksum := crc64.Checksum([]byte(expectedText), table)

	tailer := suite.createTailer()

	fingerprinter := NewFingerprinter(true, *config)
	receivedChecksum := fingerprinter.ComputeFingerprint(tailer.file)

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

	fingerprinter := NewFingerprinter(true, *config)
	receivedChecksum := fingerprinter.ComputeFingerprint(tailer.file)

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

	fingerprinter := NewFingerprinter(true, *config)
	receivedChecksum := fingerprinter.ComputeFingerprint(tailer.file)
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

	fingerprinter := NewFingerprinter(true, *config)
	receivedChecksum := fingerprinter.ComputeFingerprint(tailer.file)
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
	fingerprinter := NewFingerprinter(true, *config)
	receivedChecksum := fingerprinter.ComputeFingerprint(tailer.file)
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

	fingerprinter := NewFingerprinter(true, *config)
	receivedChecksum := fingerprinter.ComputeFingerprint(tailer.file)
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

	fingerprinter := NewFingerprinter(true, *config)
	receivedChecksum := fingerprinter.ComputeFingerprint(tailer.file)
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

	fingerprinter := NewFingerprinter(true, *config)
	receivedChecksum := fingerprinter.ComputeFingerprint(tailer.file)
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

	fingerprinter := NewFingerprinter(true, *config)
	receivedChecksum := fingerprinter.ComputeFingerprint(tailer.file)
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

	fingerprinter := NewFingerprinter(true, *config)
	receivedChecksum := fingerprinter.ComputeFingerprint(tailer.file)
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

	fingerprinter := NewFingerprinter(true, *config)
	receivedChecksum := fingerprinter.ComputeFingerprint(tailer.file)
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
	fingerprinter := NewFingerprinter(true, *config)
	fingerprint := fingerprinter.ComputeFingerprint(tailer.file)

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
	fingerprinter := NewFingerprinter(true, *config)
	fingerprint := fingerprinter.ComputeFingerprint(tailer.file)

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

	fingerprinter := NewFingerprinter(true, *config)
	fingerprint := fingerprinter.ComputeFingerprint(tailer.file)

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

	fingerprinter = NewFingerprinter(true, *config)
	fingerprint = fingerprinter.ComputeFingerprint(tailer.file)

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

	fingerprinter := NewFingerprinter(true, *config)
	fingerprint := fingerprinter.ComputeFingerprint(tailer.file)

	expectedText := strings.Repeat("X", 80)
	table := crc64.MakeTable(crc64.ISO)
	expectedHash := crc64.Checksum([]byte(expectedText), table)
	// Should still compute a fingerprint even with truncation
	suite.Equal(expectedHash, fingerprint.Value)
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

	config := &types.FingerprintConfig{
		Count:               maxLines,
		CountToSkip:         linesToSkip,
		MaxBytes:            maxBytes,
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
	}

	tailer := suite.createTailer()

	fingerprinter := NewFingerprinter(true, *config)
	fingerprint := fingerprinter.ComputeFingerprint(tailer.file)

	fmt.Println(lines)
	stringToHash := strings.Repeat("A", 30) + strings.Repeat("B", 30) + strings.Repeat("C", 18)
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

	fingerprinter := NewFingerprinter(true, *fpConfig)
	fingerprint1 := fingerprinter.ComputeFingerprint(tailer.file)

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

	fingerprinter = NewFingerprinter(true, *fpConfig)
	fingerprint2 := fingerprinter.ComputeFingerprint(tailer.file)

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

	fingerprinter := NewFingerprinter(true, *config)
	receivedChecksum := fingerprinter.ComputeFingerprint(tailer.file)

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
	fingerprinter := NewFingerprinter(true, *config)

	// Compute initial fingerprint
	initialFingerprint := fingerprinter.ComputeFingerprint(tailer.file)
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

	// 3. Truncate the file, which simulates a rotation. This should be detected.
	suite.T().Log("Truncating file to simulate rotation")
	suite.Nil(suite.testFile.Truncate(0))
	_, err = suite.testFile.Seek(0, 0)
	suite.Nil(err)
	suite.Nil(suite.testFile.Sync())
	rotated, err = tailer.DidRotateViaFingerprint(fingerprinter)
	suite.Nil(err)
	suite.True(rotated, "Should detect rotation after truncation")

	// 4. Simulate a full file replacement (e.g. logrotate with 'create' directive).
	suite.T().Log("Simulating file replacement with different content")
	_, err = suite.testFile.WriteString("a completely new file\n")
	suite.Nil(err)
	suite.Nil(suite.testFile.Sync())

	// We 're-arm' the tailer, as if the launcher had picked up the new file.
	// This tailer now considers the current content ("a completely new file") as its baseline.
	tailer = suite.createTailer()
	fingerprinter = NewFingerprinter(true, *config)
	newFingerprint := fingerprinter.ComputeFingerprint(tailer.file)
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
	receivedChecksum := fingerprinter.ComputeFingerprint(tailer.file)
	suite.Equal(expectedChecksum, receivedChecksum.Value)

	// 5. Test case with an an empty file.
	// The initial fingerprint will be nil.
	suite.T().Log("Testing rotation detection with an initially empty file")
	suite.Nil(suite.testFile.Truncate(0))
	_, err = suite.testFile.Seek(0, 0)
	suite.Nil(err)
	suite.Nil(suite.testFile.Sync())
	tailer = suite.createTailer()
	fingerprinter = NewFingerprinter(true, *config)
	emptyFingerprint := fingerprinter.ComputeFingerprint(tailer.file)
	suite.Equal(uint64(0), emptyFingerprint.Value, "Fingerprint of an empty file should have Value=0")

	// Set the fingerprint on the tailer (even though it's nil)
	tailer.fingerprint = emptyFingerprint

	// `DidRotateViaFingerprint` is designed to return `false` if the original
	// fingerprint was nil, to avoid false positives.
	rotated, err = tailer.DidRotateViaFingerprint(fingerprinter)
	suite.Nil(err)
	suite.False(rotated, "Should not detect rotation if the initial fingerprint was nil")
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
	fingerprinter := NewFingerprinter(true, *config)
	fingerprint := fingerprinter.ComputeFingerprint(tailer.file)

	// Since we're trying to skip more lines than exist, and the LimitedReader exhausts,
	// this should trigger the fallback to byte-based fingerprinting
	// The fallback should read from the beginning of the file (after any byte skip)

	// Expected: the new implementation returns fingerprint with Value=0 when there's insufficient data
	suite.Equal(uint64(0), fingerprint.Value, "Should return fingerprint with Value=0 when there's insufficient data for fingerprinting")
}
