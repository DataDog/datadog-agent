// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"bufio"
	"hash/crc64"
	"io"
	"os"

	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Fingerprinter is a struct that contains the fingerprinting configuration
type Fingerprinter struct {
	fingerprintingEnabled    bool
	defaultFingerprintConfig *types.FingerprintConfig
}

// Fallback fingerprint configs used when requested fingerprint
// strategy (bytes or line-based) can not be used.
var defaultBytesConfig = &types.FingerprintConfig{
	FingerprintStrategy: types.FingerprintStrategyByteChecksum,
	Count:               1024,
	CountToSkip:         0,
}

// DefaultLinesConfig provides a sensible default configuration for line-based fingerprinting
var defaultLinesConfig = &types.FingerprintConfig{
	FingerprintStrategy: types.FingerprintStrategyLineChecksum,
	Count:               1,
	CountToSkip:         0,
	MaxBytes:            10000,
}

// newInvalidFingerprint returns a fingerprint with Value=0 to represent an invalid/empty fingerprint
func newInvalidFingerprint(config *types.FingerprintConfig) *types.Fingerprint {
	return &types.Fingerprint{Value: types.InvalidFingerprintValue, Config: config}
}

// crc64Table is a package-level variable for the CRC64 ISO table
// to avoid recreating it on every fingerprint computation
var crc64Table = crc64.MakeTable(crc64.ISO)

// NewFingerprinter creates a new Fingerprinter with the given configuration
func NewFingerprinter(fingerprintEnabled bool, defaultFingerprintConfig *types.FingerprintConfig) *Fingerprinter {
	log.Debugf("Creating new fingerprinter: enabled=%t, defaultConfig=%+v", fingerprintEnabled, defaultFingerprintConfig)
	return &Fingerprinter{
		fingerprintingEnabled:    fingerprintEnabled,
		defaultFingerprintConfig: defaultFingerprintConfig,
	}
}

// IsFingerprintingEnabled returns whether or not our configuration has checksum fingerprinting enabled
func (f *Fingerprinter) IsFingerprintingEnabled() bool {
	return f.fingerprintingEnabled
}

// ShouldFileFingerprint returns whether or not a given file should be fingerprinted to detect rotation and truncation
func (f *Fingerprinter) ShouldFileFingerprint(file *File) bool {
	if !f.fingerprintingEnabled {
		log.Debugf("Fingerprinting disabled globally, skipping file %s", file.Path)
		return false
	}

	if file.Source.Config().FingerprintConfig == nil && f.defaultFingerprintConfig == nil {
		log.Debugf("No fingerprint config available for file %s (source config: %+v, default config: %+v)",
			file.Path,
			file.Source.Config().FingerprintConfig,
			f.defaultFingerprintConfig)
		return false
	}

	log.Debugf("File %s will be fingerprinted", file.Path)
	return true
}

// ComputeFingerprintFromConfig computes the fingerprint for the given file path using a specific config
func (f *Fingerprinter) ComputeFingerprintFromConfig(filepath string, fingerprintConfig *types.FingerprintConfig) *types.Fingerprint {
	if !f.fingerprintingEnabled {
		return newInvalidFingerprint(nil)
	}
	return computeFingerprint(filepath, fingerprintConfig)
}

// ComputeFingerprint computes the fingerprint for the given file path
func (f *Fingerprinter) ComputeFingerprint(file *File) *types.Fingerprint {
	if !f.fingerprintingEnabled {
		log.Debugf("Fingerprinting disabled, returning invalid fingerprint")
		return newInvalidFingerprint(nil)
	}
	if file == nil {
		log.Warnf("file is nil, skipping fingerprinting")
		return newInvalidFingerprint(nil)
	}

	configFingerprintConfig := file.Source.Config().FingerprintConfig
	if configFingerprintConfig == nil {
		if f.defaultFingerprintConfig == nil {
			log.Debugf("No fingerprint config found in file source or defaults, returning invalid fingerprint")
			return newInvalidFingerprint(nil)
		}
		// Use the default config directly since it's already the right type
		log.Debugf("Using default fingerprint config: strategy=%s, count=%d, countToSkip=%d, maxBytes=%d",
			f.defaultFingerprintConfig.FingerprintStrategy,
			f.defaultFingerprintConfig.Count,
			f.defaultFingerprintConfig.CountToSkip,
			f.defaultFingerprintConfig.MaxBytes)
		return computeFingerprint(file.Path, f.defaultFingerprintConfig)
	}

	// Convert from config.FingerprintConfig to types.FingerprintConfig
	fingerprintConfig := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategy(configFingerprintConfig.FingerprintStrategy),
		Count:               configFingerprintConfig.Count,
		CountToSkip:         configFingerprintConfig.CountToSkip,
		MaxBytes:            configFingerprintConfig.MaxBytes,
	}

	log.Debugf("Using file source fingerprint config: strategy=%s, count=%d, countToSkip=%d, maxBytes=%d",
		fingerprintConfig.FingerprintStrategy,
		fingerprintConfig.Count,
		fingerprintConfig.CountToSkip,
		fingerprintConfig.MaxBytes)

	return computeFingerprint(file.Path, fingerprintConfig)
}

// computeFingerprint computes the fingerprint for the given file path
func computeFingerprint(filePath string, fingerprintConfig *types.FingerprintConfig) *types.Fingerprint {
	if fingerprintConfig == nil {
		log.Debugf("No fingerprint config provided, returning invalid fingerprint")
		return newInvalidFingerprint(nil)
	}

	log.Debugf("Computing fingerprint for %s with config: strategy=%s, count=%d, countToSkip=%d, maxBytes=%d",
		filePath,
		fingerprintConfig.FingerprintStrategy,
		fingerprintConfig.Count,
		fingerprintConfig.CountToSkip,
		fingerprintConfig.MaxBytes)

	fpFile, err := os.Open(filePath)
	if err != nil {
		log.Warnf("could not open file for fingerprinting %s: %v", filePath, err)
		return newInvalidFingerprint(fingerprintConfig)
	}
	defer fpFile.Close()

	// Determine fingerprinting strategy (line_checksum or byte_checksum)
	strategy := fingerprintConfig.FingerprintStrategy
	switch strategy {
	case types.FingerprintStrategyLineChecksum:
		log.Debugf("Using line-based fingerprinting strategy for %s", filePath)
		return computeFingerPrintByLines(fpFile, filePath, fingerprintConfig)
	case types.FingerprintStrategyByteChecksum:
		log.Debugf("Using byte-based fingerprinting strategy for %s", filePath)
		return computeFingerPrintByBytes(fpFile, filePath, fingerprintConfig)
	default:
		log.Warnf("invalid fingerprint strategy %q for file %q, using default lines strategy: %v", strategy, filePath, err)
		// Default to line_checksum if no strategy is specified
		log.Debugf("Falling back to default line-based strategy for %s", filePath)
		return computeFingerPrintByLines(fpFile, filePath, defaultLinesConfig)
	}
}

// computeFingerPrintByBytes computes fingerprint using byte-based approach for a given file path
func computeFingerPrintByBytes(fpFile *os.File, filePath string, fingerprintConfig *types.FingerprintConfig) *types.Fingerprint {
	bytesToSkip := fingerprintConfig.CountToSkip
	maxBytes := fingerprintConfig.Count
	if fingerprintConfig.FingerprintStrategy == "line_checksum" {
		bytesToSkip = 0
		maxBytes = fingerprintConfig.MaxBytes
	}
	// Skip the configured number of bytes
	if bytesToSkip > 0 {
		_, err := fpFile.Seek(int64(bytesToSkip), io.SeekStart)

		if err != nil {
			log.Warnf("Failed to skip %d bytes while computing fingerprint for %q: %v", bytesToSkip, filePath, err)
			return newInvalidFingerprint(fingerprintConfig)
		}
	}

	// Read up to maxBytes for hashing
	buffer := make([]byte, maxBytes)
	bytesRead, err := io.ReadFull(fpFile, buffer)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		log.Warnf("Failed to read bytes for fingerprint %q: %v", filePath, err)
		return newInvalidFingerprint(fingerprintConfig)
	}

	// Trim buffer to actual bytes read
	buffer = buffer[:bytesRead]

	// Check if we have enough bytes to create a meaningful fingerprint
	if bytesRead == 0 || bytesRead < maxBytes {
		log.Debugf("No bytes available for fingerprinting file %q", filePath)
		return newInvalidFingerprint(fingerprintConfig)
	}

	// Compute fingerprint
	checksum := crc64.Checksum(buffer, crc64Table)

	log.Debugf("Computed byte-based fingerprint 0x%x for file %q (bytes=%d)", checksum, filePath, bytesRead)
	return &types.Fingerprint{Value: checksum, Config: fingerprintConfig}
}

// computeFingerPrintByLines computes fingerprint using line-based approach for a given file path
func computeFingerPrintByLines(fpFile *os.File, filePath string, fingerprintConfig *types.FingerprintConfig) *types.Fingerprint {
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
		return newInvalidFingerprint(fingerprintConfig)
	}

	// Check if we have enough lines to create a meaningful fingerprint
	if linesRead == 0 {
		if scanner.Err() == nil && limitedReader.(*io.LimitedReader).N == 0 {
			// Scanner stopped normally + no bytes remaining = fall back
			log.Warnf("Scanner stopped with no bytes remaining, falling back to byte-based fingerprint for %q", filePath)
			pos, err := fpFile.Seek(0, io.SeekStart)
			if pos != 0 || err != nil {
				log.Warnf("Error %s occurred while trying to reset file offset", err)
			}
			return computeFingerPrintByBytes(fpFile, filePath, defaultBytesConfig)
		}
	}

	if linesRead < maxLines && bytesRead < maxBytes {
		log.Debugf("Not enough data for fingerprinting file %q (lines=%d, bytes=%d)", filePath, linesRead, bytesRead)
		return newInvalidFingerprint(fingerprintConfig)
	}
	// Compute fingerprint
	checksum := crc64.Checksum(buffer, crc64Table)
	log.Debugf("Computed line-based fingerprint 0x%x for file %q (bytes=%d, lines=%d)", checksum, filePath, len(buffer), linesRead)
	return &types.Fingerprint{Value: checksum, Config: fingerprintConfig}
}
