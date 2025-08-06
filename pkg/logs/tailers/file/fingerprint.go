// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"bufio"
	"fmt"
	"hash/crc64"
	"io"
	"os"

	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// fallbackFingerprintConfig is the fallback fingerprint config used when requested fingerprint
// strategy can not be used.
var defaultBytesConfig = &logsconfig.FingerprintConfig{
	FingerprintStrategy: logsconfig.FingerprintStrategyByteChecksum,
	Count:               1024,
	CountToSkip:         0,
}

var defaultLinesConfig = &logsconfig.FingerprintConfig{
	FingerprintStrategy: logsconfig.FingerprintStrategyLineChecksum,
	Count:               1,
	CountToSkip:         0,
	MaxBytes:            10000,
}

const (
	InvalidFingerprintValue = 0
)

type Fingerprint struct {
	Value  uint64
	Config *logsconfig.FingerprintConfig
}

func (f *Fingerprint) String() string {
	return fmt.Sprintf("Fingerprint{Value: %d, Config: %v}", f.Value, f.Config)
}

func (f *Fingerprint) Equals(other *Fingerprint) bool {
	return f.Value == other.Value
}

func (f *Fingerprint) ValidFingerprint() bool {
	return f.Value != InvalidFingerprintValue && f.Config != nil
}

// crc64Table is a package-level variable for the CRC64 ISO table
// to avoid recreating it on every fingerprint computation
var crc64Table = crc64.MakeTable(crc64.ISO)

type Fingerprinter struct {
	fingerprintEnabled       bool
	defaultFingerprintConfig *logsconfig.FingerprintConfig
}

func NewFingerprinter(fingerprintEnabled bool, defaultFingerprintConfig *logsconfig.FingerprintConfig) *Fingerprinter {
	return &Fingerprinter{
		fingerprintEnabled:       fingerprintEnabled,
		defaultFingerprintConfig: defaultFingerprintConfig,
	}
}

func (f *Fingerprinter) IsFingerprintingEnabled() bool {
	return f.fingerprintEnabled
}

func (f *Fingerprinter) ShouldFileFingerprint(file *File) bool {
	if !f.fingerprintEnabled {
		return false
	}

	if file.Source.Config().FingerprintConfig == nil && f.defaultFingerprintConfig == nil {
		return false
	}

	return true
}

func (f *Fingerprinter) ComputeFingerprintFromConfig(filepath string, fingerprintConfig *logsconfig.FingerprintConfig) *Fingerprint {
	if !f.fingerprintEnabled {
		return nil
	}
	return computeFingerprint(filepath, fingerprintConfig)
}

// ComputeFingerprint computes the fingerprint for the given file path
func (f *Fingerprinter) ComputeFingerprint(file *File) *Fingerprint {
	if !f.fingerprintEnabled {
		return nil
	}
	if file == nil {
		log.Warnf("file is nil, skipping fingerprinting")
		return nil
	}

	fingerprintConfig := file.Source.Config().FingerprintConfig
	if fingerprintConfig == nil {
		if f.defaultFingerprintConfig == nil {
			return nil
		}
		fingerprintConfig = f.defaultFingerprintConfig
	}

	return computeFingerprint(file.Path, fingerprintConfig)
}

func computeFingerprint(filePath string, fingerprintConfig *logsconfig.FingerprintConfig) *Fingerprint {
	if fingerprintConfig == nil {
		return nil
	}

	fpFile, err := os.Open(filePath)
	if err != nil {
		log.Warnf("could not open file for fingerprinting %s: %v", filePath, err)
		return nil
	}
	defer fpFile.Close()

	// Determine fingerprinting strategy
	strategy := fingerprintConfig.FingerprintStrategy
	switch strategy {
	case logsconfig.FingerprintStrategyLineChecksum:
		return computeFingerPrintByLines(fpFile, filePath, fingerprintConfig)
	case logsconfig.FingerprintStrategyByteChecksum:
		return computeFingerPrintByBytes(fpFile, filePath, fingerprintConfig)
	default:
		log.Warnf("invalid fingerprint strategy %q for file %q, using default lines strategy: %v", strategy, filePath, err)
		// Default to line_checksum if no strategy is specified
		return computeFingerPrintByLines(fpFile, filePath, defaultLinesConfig)
	}
}

// computeFileFingerPrintByBytes computes fingerprint using byte-based approach for a given file path
func computeFingerPrintByBytes(fpFile *os.File, filePath string, fingerprintConfig *logsconfig.FingerprintConfig) *Fingerprint {
	bytesToSkip := fingerprintConfig.CountToSkip
	maxBytes := fingerprintConfig.Count

	// Skip the configured number of bytes
	if bytesToSkip > 0 {
		_, err := fpFile.Seek(int64(bytesToSkip), io.SeekStart)

		if err != nil {
			log.Warnf("Failed to skip %d bytes while computing fingerprint for %q: %v", bytesToSkip, filePath, err)
			return nil
		}
	}

	// Read up to maxBytes for hashing
	buffer := make([]byte, maxBytes)
	bytesRead, err := io.ReadFull(fpFile, buffer)
	if err != nil {
		log.Warnf("Failed to read sufficient bytes for fingerprint %q: %v", filePath, err)
		return nil
	}

	// Compute fingerprint
	checksum := crc64.Checksum(buffer, crc64Table)

	log.Debugf("Computed byte-based fingerprint 0x%x for file %q (bytes=%d)", checksum, filePath, bytesRead)
	return &Fingerprint{Value: checksum, Config: fingerprintConfig}
}

// computeFileFingerPrintByLines computes fingerprint using line-based approach for a given file path
func computeFingerPrintByLines(fpFile *os.File, filePath string, fingerprintConfig *logsconfig.FingerprintConfig) *Fingerprint {
	linesToSkip := fingerprintConfig.CountToSkip
	maxLines := fingerprintConfig.Count
	maxBytes := fingerprintConfig.MaxBytes

	// Create a LimitedReader to respect maxBytes constraint
	limitedReader := io.LimitReader(fpFile, int64(maxBytes))

	// Create scanner for line-by-line reading
	scanner := bufio.NewScanner(limitedReader)

	// Skip the configured number of lines
	for i := 0; i < linesToSkip; i++ {
		if !scanner.Scan() {
			if scanner.Err() != nil {
				log.Warnf("Failed to skip line while computing fingerprint for %q: %v", filePath, scanner.Err())
			}
			if scanner.Err() == nil && limitedReader.(*io.LimitedReader).N == 0 {
				// Scanner stopped normally + no bytes remaining = fall back
				log.Warnf("Scanner stopped with no bytes remaining, falling back to bytes fingerprint for %q", filePath)
				pos, err := fpFile.Seek(0, io.SeekStart)
				if pos != 0 || err != nil {
					log.Warnf("Error %s occurred while trying to reset file offset", err)
					return nil
				}
				return computeFingerPrintByBytes(fpFile, filePath, defaultBytesConfig)
			}
		}
	}

	// Read lines for hashing
	var buffer []byte
	linesRead := 0
	bytesRead := 0
	for linesRead < maxLines && scanner.Scan() {
		//TODO: Check for error here
		line := scanner.Bytes()
		buffer = append(buffer, line...)
		linesRead++
		bytesRead += maxBytes - int(limitedReader.(*io.LimitedReader).N)

		// Note: scanner.Bytes() strips newline characters, so bytesRead only includes
		// the actual line content, not the newline characters. This is a limitation
		// of using bufio.Scanner for line-based reading.
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		log.Warnf("Error while reading file for fingerprint %q: %v", filePath, err)
		return nil
	}

	// Check if we have enough lines to create a meaningful fingerprint
	if linesRead == 0 {
		if scanner.Err() == nil && limitedReader.(*io.LimitedReader).N == 0 {
			// Scanner stopped normally + no bytes remaining = fall back
			log.Warnf("Scanner stopped with no bytes remaining, falling back to byte-based fingerprint for %q", filePath)
			pos, err := fpFile.Seek(0, io.SeekStart)
			if pos != 0 || err != nil {
				log.Warnf("Error %s occurred while trying to reset file offset", err)
				return nil
			}
			return computeFingerPrintByBytes(fpFile, filePath, defaultBytesConfig)
		}
	}

	if linesRead < maxLines && bytesRead < maxBytes {
		log.Debugf("Not enough data for fingerprinting file %q (lines=%d, bytes=%d)", filePath, linesRead, bytesRead)
		return nil
	}
	// Compute fingerprint
	checksum := crc64.Checksum(buffer, crc64Table)
	log.Debugf("Computed line-based fingerprint 0x%x for file %q (bytes=%d, lines=%d)", checksum, filePath, len(buffer), linesRead)
	return &Fingerprint{Value: checksum, Config: fingerprintConfig}
}
