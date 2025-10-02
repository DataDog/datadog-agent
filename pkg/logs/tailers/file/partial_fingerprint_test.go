// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	logstypes "github.com/DataDog/datadog-agent/pkg/logs/types"
)

type PartialFingerprintTestSuite struct {
	suite.Suite
	testDir    string
	outputChan chan *message.Message
	source     *sources.ReplaceableSource
}

func (suite *PartialFingerprintTestSuite) SetupTest() {
	suite.testDir = suite.T().TempDir()
	suite.outputChan = make(chan *message.Message, 10)

	// Create a source with fingerprint config
	logConfig := &config.LogsConfig{
		Type: config.FileType,
		Path: filepath.Join(suite.testDir, "test.log"),
		FingerprintConfig: &logstypes.FingerprintConfig{
			FingerprintStrategy: logstypes.FingerprintStrategyByteChecksum,
			Count:               1024,
			CountToSkip:         0,
		},
	}

	suite.source = sources.NewReplaceableSource(sources.NewLogSource("test", logConfig))
}

func TestPartialFingerprintSuite(t *testing.T) {
	suite.Run(t, new(PartialFingerprintTestSuite))
}

// TestTailerStartsInPartialFingerprintState tests that a tailer starts in partial fingerprint state
// when the file is empty or doesn't have enough data for a fingerprint
func (suite *PartialFingerprintTestSuite) TestTailerStartsInPartialFingerprintState() {
	testPath := filepath.Join(suite.testDir, "empty.log")
	f, err := os.Create(testPath)
	suite.Require().NoError(err)
	defer f.Close()

	file := NewFile(testPath, suite.source.UnderlyingSource(), false)

	// Create tailer with nil fingerprint (empty file)
	info := status.NewInfoRegistry()
	opts := &TailerOptions{
		OutputChan:      suite.outputChan,
		File:            file,
		SleepDuration:   10 * time.Millisecond,
		Decoder:         decoder.NewDecoderFromSource(suite.source, info),
		Info:            info,
		Fingerprint:     nil, // No fingerprint for empty file
		Registry:        auditor.NewMockRegistry(),
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
	}

	tailer := NewTailer(opts)

	// Verify tailer is in partial fingerprint state
	suite.True(tailer.isPartialFingerprintState.Load(), "Tailer should start in partial fingerprint state")
	suite.NotNil(tailer.fingerprintBuffer, "Fingerprint buffer should be allocated")
	suite.Equal(1024, tailer.fingerprintBufferSize, "Buffer size should match config")
	suite.Equal(0, tailer.fingerprintBytesToSkip, "Bytes to skip should match config")
	suite.Equal(0, tailer.fingerprintBufferOffset, "Buffer offset should start at 0")
}

// TestAccumulateDataInBuffer tests that data is accumulated in the buffer
func (suite *PartialFingerprintTestSuite) TestAccumulateDataInBuffer() {
	testPath := filepath.Join(suite.testDir, "test.log")
	f, err := os.Create(testPath)
	suite.Require().NoError(err)
	defer f.Close()

	file := NewFile(testPath, suite.source.UnderlyingSource(), false)

	info := status.NewInfoRegistry()
	opts := &TailerOptions{
		OutputChan:      suite.outputChan,
		File:            file,
		SleepDuration:   10 * time.Millisecond,
		Decoder:         decoder.NewDecoderFromSource(suite.source, info),
		Info:            info,
		Fingerprint:     nil,
		Registry:        auditor.NewMockRegistry(),
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
	}

	tailer := NewTailer(opts)

	// Accumulate some data
	testData := []byte("hello world\n")
	tailer.accumulateForFingerprint(testData)

	// Verify data was accumulated
	suite.Equal(len(testData), tailer.fingerprintBufferOffset, "Buffer offset should advance")
	suite.Equal(testData, tailer.fingerprintBuffer[:len(testData)], "Data should be stored in buffer")
	suite.True(tailer.isPartialFingerprintState.Load(), "Should still be in partial state")
}

// TestComputeFingerprintWhenBufferFull tests that fingerprint is computed when buffer is full
func (suite *PartialFingerprintTestSuite) TestComputeFingerprintWhenBufferFull() {
	testPath := filepath.Join(suite.testDir, "full.log")
	f, err := os.Create(testPath)
	suite.Require().NoError(err)
	defer f.Close()

	file := NewFile(testPath, suite.source.UnderlyingSource(), false)

	info := status.NewInfoRegistry()
	opts := &TailerOptions{
		OutputChan:      suite.outputChan,
		File:            file,
		SleepDuration:   10 * time.Millisecond,
		Decoder:         decoder.NewDecoderFromSource(suite.source, info),
		Info:            info,
		Fingerprint:     nil,
		Registry:        auditor.NewMockRegistry(),
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
	}

	tailer := NewTailer(opts)

	// Fill the buffer
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	tailer.accumulateForFingerprint(data)

	// Verify fingerprint was computed and buffer cleared
	suite.False(tailer.isPartialFingerprintState.Load(), "Should exit partial state when buffer full")
	suite.Nil(tailer.fingerprintBuffer, "Buffer should be cleared")
	suite.NotNil(tailer.fingerprint, "Fingerprint should be set")
	suite.Equal(1024, tailer.fingerprint.BytesUsed, "Should use full buffer")
	suite.NotEqual(uint64(0), tailer.fingerprint.Value, "Fingerprint value should be non-zero")
}

// TestGetPartialFingerprintFromBuffer tests getting a temporary partial fingerprint
func (suite *PartialFingerprintTestSuite) TestGetPartialFingerprintFromBuffer() {
	testPath := filepath.Join(suite.testDir, "partial.log")
	f, err := os.Create(testPath)
	suite.Require().NoError(err)
	defer f.Close()

	file := NewFile(testPath, suite.source.UnderlyingSource(), false)

	info := status.NewInfoRegistry()
	opts := &TailerOptions{
		OutputChan:      suite.outputChan,
		File:            file,
		SleepDuration:   10 * time.Millisecond,
		Decoder:         decoder.NewDecoderFromSource(suite.source, info),
		Info:            info,
		Fingerprint:     nil,
		Registry:        auditor.NewMockRegistry(),
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
	}

	tailer := NewTailer(opts)

	// Accumulate partial data (less than full buffer)
	testData := []byte("partial data\n")
	tailer.accumulateForFingerprint(testData)

	// Get partial fingerprint
	partialFP := tailer.getPartialFingerprintFromBuffer()

	// Verify partial fingerprint was computed
	suite.NotNil(partialFP, "Should return partial fingerprint")
	suite.True(partialFP.IsPartialFingerprint(), "Should be marked as partial")
	suite.Equal(len(testData), partialFP.BytesUsed, "BytesUsed should match accumulated data")
	suite.NotEqual(uint64(0), partialFP.Value, "Should have non-zero checksum")

	// Verify buffer is NOT cleared and state is unchanged
	suite.True(tailer.isPartialFingerprintState.Load(), "Should still be in partial state")
	suite.NotNil(tailer.fingerprintBuffer, "Buffer should NOT be cleared")
	suite.Equal(len(testData), tailer.fingerprintBufferOffset, "Buffer offset should be unchanged")
}

// TestPartialFingerprintWithSkipBytes tests partial fingerprint with skip bytes configured
func (suite *PartialFingerprintTestSuite) TestPartialFingerprintWithSkipBytes() {
	testPath := filepath.Join(suite.testDir, "skip.log")
	f, err := os.Create(testPath)
	suite.Require().NoError(err)
	defer f.Close()

	// Create source with skip bytes
	logConfig := &config.LogsConfig{
		Type: config.FileType,
		Path: testPath,
		FingerprintConfig: &logstypes.FingerprintConfig{
			FingerprintStrategy: logstypes.FingerprintStrategyByteChecksum,
			Count:               100,
			CountToSkip:         10, // Skip first 10 bytes
		},
	}

	source := sources.NewReplaceableSource(sources.NewLogSource("test-skip", logConfig))
	file := NewFile(testPath, source.UnderlyingSource(), false)

	info := status.NewInfoRegistry()
	opts := &TailerOptions{
		OutputChan:      suite.outputChan,
		File:            file,
		SleepDuration:   10 * time.Millisecond,
		Decoder:         decoder.NewDecoderFromSource(source, info),
		Info:            info,
		Fingerprint:     nil,
		Registry:        auditor.NewMockRegistry(),
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
	}

	tailer := NewTailer(opts)

	// Accumulate data
	data := []byte("0123456789ABCDEFGHIJ") // 20 bytes total
	tailer.accumulateForFingerprint(data)

	// Get partial fingerprint
	partialFP := tailer.getPartialFingerprintFromBuffer()

	// Verify: should skip first 10 bytes, use next 10 bytes
	suite.NotNil(partialFP)
	suite.Equal(10, partialFP.BytesUsed, "Should use 10 bytes after skipping 10")

	// Buffer should contain "ABCDEFGHIJ" at the beginning
	suite.Equal([]byte("ABCDEFGHIJ"), tailer.fingerprintBuffer[:10])
}

// TestRotationDetectionSameSizeDifferentChecksum tests rotation detection when
// partial fingerprints have same size but different checksums
func (suite *PartialFingerprintTestSuite) TestRotationDetectionSameSizeDifferentChecksum() {
	testPath := filepath.Join(suite.testDir, "rotate1.log")

	// Create file with initial content
	f, err := os.Create(testPath)
	suite.Require().NoError(err)
	_, err = f.WriteString("initial data\n")
	suite.Require().NoError(err)
	f.Close()

	file := NewFile(testPath, suite.source.UnderlyingSource(), false)
	fingerprinter := NewFingerprinter(logstypes.FingerprintConfig{
		FingerprintStrategy: logstypes.FingerprintStrategyByteChecksum,
		Count:               1024,
		CountToSkip:         0,
	})

	info := status.NewInfoRegistry()
	opts := &TailerOptions{
		OutputChan:      suite.outputChan,
		File:            file,
		SleepDuration:   10 * time.Millisecond,
		Decoder:         decoder.NewDecoderFromSource(suite.source, info),
		Info:            info,
		Fingerprint:     nil,
		Registry:        auditor.NewMockRegistry(),
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
	}

	tailer := NewTailer(opts)

	// Accumulate some data
	tailer.accumulateForFingerprint([]byte("initial data\n"))

	// Now "rotate" the file by replacing it with different content of same size
	os.Remove(testPath)
	f, err = os.Create(testPath)
	suite.Require().NoError(err)
	_, err = f.WriteString("different!!!\n") // Same size, different content
	suite.Require().NoError(err)
	f.Close()

	// Check rotation
	rotated, err := tailer.DidRotateViaFingerprint(fingerprinter)
	suite.NoError(err)
	suite.True(rotated, "Should detect rotation when partial fingerprints differ")
}

// TestRotationDetectionFileShrunk tests rotation detection when file shrinks
func (suite *PartialFingerprintTestSuite) TestRotationDetectionFileShrunk() {
	testPath := filepath.Join(suite.testDir, "rotate2.log")

	// Create file with larger content
	f, err := os.Create(testPath)
	suite.Require().NoError(err)
	_, err = f.WriteString("this is some longer initial data\n")
	suite.Require().NoError(err)
	f.Close()

	file := NewFile(testPath, suite.source.UnderlyingSource(), false)
	fingerprinter := NewFingerprinter(logstypes.FingerprintConfig{
		FingerprintStrategy: logstypes.FingerprintStrategyByteChecksum,
		Count:               1024,
		CountToSkip:         0,
	})

	info := status.NewInfoRegistry()
	opts := &TailerOptions{
		OutputChan:      suite.outputChan,
		File:            file,
		SleepDuration:   10 * time.Millisecond,
		Decoder:         decoder.NewDecoderFromSource(suite.source, info),
		Info:            info,
		Fingerprint:     nil,
		Registry:        auditor.NewMockRegistry(),
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
	}

	tailer := NewTailer(opts)

	// Accumulate data
	tailer.accumulateForFingerprint([]byte("this is some longer initial data\n"))

	// "Rotate" by replacing with smaller file
	os.Remove(testPath)
	f, err = os.Create(testPath)
	suite.Require().NoError(err)
	_, err = f.WriteString("short\n") // Much smaller
	suite.Require().NoError(err)
	f.Close()

	// Check rotation
	rotated, err := tailer.DidRotateViaFingerprint(fingerprinter)
	suite.NoError(err)
	suite.True(rotated, "Should detect rotation when file shrinks")
}

// TestRotationDetectionFileGrew tests rotation detection when file grows
func (suite *PartialFingerprintTestSuite) TestRotationDetectionFileGrew() {
	testPath := filepath.Join(suite.testDir, "rotate3.log")

	// Create file with smaller content
	f, err := os.Create(testPath)
	suite.Require().NoError(err)
	_, err = f.WriteString("small\n")
	suite.Require().NoError(err)
	f.Close()

	file := NewFile(testPath, suite.source.UnderlyingSource(), false)
	fingerprinter := NewFingerprinter(logstypes.FingerprintConfig{
		FingerprintStrategy: logstypes.FingerprintStrategyByteChecksum,
		Count:               1024,
		CountToSkip:         0,
	})

	info := status.NewInfoRegistry()
	opts := &TailerOptions{
		OutputChan:      suite.outputChan,
		File:            file,
		SleepDuration:   10 * time.Millisecond,
		Decoder:         decoder.NewDecoderFromSource(suite.source, info),
		Info:            info,
		Fingerprint:     nil,
		Registry:        auditor.NewMockRegistry(),
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
	}

	tailer := NewTailer(opts)

	// Accumulate data
	tailer.accumulateForFingerprint([]byte("small\n"))

	// File grows (no rotation - just more data written)
	f, err = os.OpenFile(testPath, os.O_APPEND|os.O_WRONLY, 0644)
	suite.Require().NoError(err)
	_, err = f.WriteString("additional data that makes file longer\n")
	suite.Require().NoError(err)
	f.Close()

	// Check rotation - should fall back to filesystem check
	// In this case, file didn't actually rotate, so should return false
	rotated, err := tailer.DidRotateViaFingerprint(fingerprinter)
	suite.NoError(err)
	suite.False(rotated, "Should not detect rotation when file just grows")
}
