// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && windows

package file

import (
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
)

// newTestTailer creates a tailer configured similarly to suite setup in tailer_test.go
// but without starting readForever; it allows direct calls to readAvailable.
func newTestTailer(t *testing.T, testPath string, fingerprint *types.Fingerprint, fingerprinter Fingerprinter, opener opener.FileOpener, decoderOptions *decoder.MockDecoderOptions) (*Tailer, chan *message.Message) {
	t.Helper()

	outputChan := make(chan *message.Message, 20)
	source := sources.NewReplaceableSource(sources.NewLogSource("", &config.LogsConfig{
		Type: config.FileType,
		Path: testPath,
	}))
	sleepDuration := 1 * time.Millisecond
	info := status.NewInfoRegistry()

	decoder := decoder.NewMockDecoderWithOptions(decoderOptions)

	tailerOptions := &TailerOptions{
		OutputChan:      outputChan,
		Decoder:         decoder,
		File:            NewFile(testPath, source.UnderlyingSource(), false),
		SleepDuration:   sleepDuration,
		Info:            info,
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
		Registry:        auditor.NewMockRegistry(),
		Fingerprint:     fingerprint,
		Fingerprinter:   fingerprinter,
		FileOpener:      opener,
	}

	tailer := NewTailer(tailerOptions)
	tailer.setup(0, io.SeekStart)

	return tailer, outputChan
}

func TestReadAvailable(t *testing.T) {
	mockFile := opener.NewMockFile("test.log", [][]byte{
		[]byte("one\ntwo\n"),
		[]byte("three\nfour\n"),
	})

	opener := opener.NewMockFileOpener()
	opener.AddMockFile(mockFile)

	fingerprinterMock := NewFingerprinterMock()
	tailer, _ := newTestTailer(t, mockFile.Name(), nil, fingerprinterMock, opener, nil)
	n, err := tailer.readAvailable()
	assert.ErrorIs(t, err, io.EOF, "Expected EOF, got %v", err)
	assert.Equal(t, mockFile.FileSize(), n, "Expected to have read the entire file of %d bytes, got %d", mockFile.FileSize(), n)

	assert.Equal(t, 2, len(tailer.decoder.InputChan()), "Expected 2 messages to have been decoded")
}

func TestReadAvailableRotation(t *testing.T) {
	mockFile := opener.NewMockFile(
		"test.log",
		[][]byte{
			[]byte("one\ntwo\n"),
			[]byte("three\nfour\n"),
		},
		[][]byte{
			[]byte("five\nsix\n"),
		},
	)

	firstFileSize := mockFile.FileSize()

	opener := opener.NewMockFileOpener()
	opener.AddMockFile(mockFile)

	mockDecoderOptions := &decoder.MockDecoderOptions{
		InputChanSize:  0,
		OutputChanSize: 0,
	}

	tailer, _ := newTestTailer(t, mockFile.Name(), nil, nil, opener, mockDecoderOptions)
	tailer.windowsOpenFileTimeout = 0

	// Consume the first chunk immediately to allow it to complete,
	// but block on the second chunk to force file close before rotation
	go func() {
		<-tailer.decoder.InputChan()      // Consume first chunk
		time.Sleep(50 * time.Millisecond) // Let the second read attempt and hit timeout
		<-tailer.decoder.InputChan()
		tailer.decoder.Start()
	}()
	n, err := tailer.readAvailable()
	assert.ErrorIs(t, err, io.EOF, "Expected EOF, got %v", err)
	assert.Equal(t, firstFileSize, n, "Expected to have read the first file of %d bytes, got %d", firstFileSize, n)
	assert.Equal(t, true, tailer.didFileRotate.Load(), "Expected file to have rotated")
}
func TestReadAvailableFingerprintMismatch(t *testing.T) {
	mockFile := opener.NewMockFile(
		"test.log",
		[][]byte{
			[]byte("one\ntwo\n"),
			[]byte("three\nfour\n"),
		},
	)
	opener := opener.NewMockFileOpener()
	opener.AddMockFile(mockFile)

	fpConfig := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
		Count:               1,
		CountToSkip:         0,
		MaxBytes:            10000,
		Source:              types.FingerprintConfigSourcePerSource,
	}
	originalFingerprint := &types.Fingerprint{Value: 1234567890, Config: fpConfig}
	fingerprinterMock := NewFingerprinterMock()
	fingerprinterMock.SetSequence(
		mockFile.Name(),
		&types.Fingerprint{Value: 6789012345, Config: fpConfig}, // Different fingerprint from original
	)

	tailer, _ := newTestTailer(t, mockFile.Name(), nil, fingerprinterMock, opener, nil)
	tailer.fingerprint = originalFingerprint
	n, err := tailer.readAvailable()

	assert.ErrorIs(t, err, io.EOF, "Expected EOF, got %v", err)
	assert.Equal(t, 0, n, "Expected 0 bytes read, got %d", n)
	assert.Equal(t, true, tailer.didFileRotate.Load(), "Expected file to have rotated")
}

func TestReadAvailableFingerprintMismatchMidRead(t *testing.T) {
	firstLine := []byte("one\ntwo\n")
	mockFile := opener.NewMockFile(
		"test.log",
		[][]byte{
			firstLine,
			[]byte("three\nfour\n"),
		},
	)

	firstLineSize := len(firstLine)

	opener := opener.NewMockFileOpener()
	opener.AddMockFile(mockFile)

	fpConfig := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
		Count:               1,
		CountToSkip:         0,
		MaxBytes:            10000,
		Source:              types.FingerprintConfigSourcePerSource,
	}
	originalFingerprint := &types.Fingerprint{Value: 1234567890, Config: fpConfig}
	fingerprinterMock := NewFingerprinterMock()
	fingerprinterMock.SetSequence(
		mockFile.Name(),
		originalFingerprint,
		&types.Fingerprint{Value: 6789012345, Config: fpConfig},
	)

	mockDecoderOptions := &decoder.MockDecoderOptions{
		InputChanSize:  0,
		OutputChanSize: 0,
	}

	tailer, _ := newTestTailer(t, mockFile.Name(), nil, fingerprinterMock, opener, mockDecoderOptions)
	tailer.fingerprint = originalFingerprint
	tailer.windowsOpenFileTimeout = 0
	go func() {
		// Wait for the decoder to attempt to read the first line
		time.Sleep(50 * time.Millisecond)
		<-tailer.decoder.InputChan()
		tailer.decoder.Start()
	}()
	n, err := tailer.readAvailable()
	assert.ErrorIs(t, err, io.EOF, "Expected EOF, got %v", err)
	assert.Equal(t, firstLineSize, n, "Expected to have read the first line of %d bytes, got %d", firstLineSize, n)
	assert.Equal(t, true, tailer.didFileRotate.Load(), "Expected file to have rotated")
}
