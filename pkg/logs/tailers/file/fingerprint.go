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
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// crc64Table is a package-level variable for the CRC64 ISO table
// to avoid recreating it on every fingerprint computation
var crc64Table = crc64.MakeTable(crc64.ISO)

// ReturnFingerprintConfig returns the configuration for the fingerprinting algorithm set by user (also used for testing)
func ReturnFingerprintConfig() *logsconfig.FingerprintConfig {
	strategy := pkgconfigsetup.Datadog().GetString("logs_config.fingerprint_strategy")
	log.Debugf("Fingerprint strategy: %s", strategy)

	if strategy != "checksum" {
		log.Debugf("Fingerprint strategy is not 'checksum', returning nil config")
		return nil
	}

	maxLines := pkgconfigsetup.Datadog().GetInt("logs_config.fingerprint_config.max_lines")
	maxBytes := pkgconfigsetup.Datadog().GetInt("logs_config.fingerprint_config.max_bytes")
	bytesToSkip := pkgconfigsetup.Datadog().GetInt("logs_config.fingerprint_config.bytes_to_skip")
	linesToSkip := pkgconfigsetup.Datadog().GetInt("logs_config.fingerprint_config.lines_to_skip")

	log.Debugf("Fingerprint config values - maxLines: %d, maxBytes: %d, bytesToSkip: %d, linesToSkip: %d",
		maxLines, maxBytes, bytesToSkip, linesToSkip)

	config := &logsconfig.FingerprintConfig{
		MaxLines:    &maxLines,
		MaxBytes:    &maxBytes,
		BytesToSkip: &bytesToSkip,
		LinesToSkip: &linesToSkip,
	}

	if err := validFingerprintConfig(config); err != nil {
		log.Warnf("Invalid fingerprint config: %v", err)
		return nil
	}

	log.Debugf("Fingerprint config is valid and will be used")
	return config
}

// validFingerprintConfig validates the fingerprint config and returns an error if the config is invalid
func validFingerprintConfig(config *logsconfig.FingerprintConfig) error {
	log.Debugf("Validating fingerprint config")
	if config == nil {
		log.Debugf("Fingerprint config is nil")
		return fmt.Errorf("fingerprint config cannot be nil")
	}

	// Check if both skip modes are set (invalid configuration)
	linesSkipSet := config.LinesToSkip != nil && *config.LinesToSkip != 0
	bytesSkipSet := config.BytesToSkip != nil && *config.BytesToSkip != 0

	log.Debugf("Skip mode validation - linesSkipSet: %v, bytesSkipSet: %v", linesSkipSet, bytesSkipSet)

	if linesSkipSet && bytesSkipSet {
		log.Debugf("Invalid configuration: both linesToSkip and bytesToSkip are set")
		return fmt.Errorf("invalid configuration: both linesToSkip and bytesToSkip are set")
	}

	// Validate non-negative fields
	if config.MaxLines != nil && *config.MaxLines < 0 {
		log.Debugf("Invalid maxLines: %d (negative)", *config.MaxLines)
		return fmt.Errorf("maxLines cannot be negative, got: %d", *config.MaxLines)
	}
	if config.BytesToSkip != nil && *config.BytesToSkip < 0 {
		log.Debugf("Invalid bytesToSkip: %d (negative)", *config.BytesToSkip)
		return fmt.Errorf("bytesToSkip cannot be negative, got: %d", *config.BytesToSkip)
	}
	if config.LinesToSkip != nil && *config.LinesToSkip < 0 {
		log.Debugf("Invalid linesToSkip: %d (negative)", *config.LinesToSkip)
		return fmt.Errorf("linesToSkip cannot be negative, got: %d", *config.LinesToSkip)
	}

	// Validate MaxBytes (must be positive)
	if config.MaxBytes != nil {
		if *config.MaxBytes < 0 {
			log.Debugf("Invalid maxBytes: %d (negative)", *config.MaxBytes)
			return fmt.Errorf("maxBytes cannot be negative, got: %d", *config.MaxBytes)
		}
		if *config.MaxBytes == 0 {
			log.Debugf("Invalid maxBytes: %d (zero)", *config.MaxBytes)
			return fmt.Errorf("maxBytes cannot be zero")
		}
	}

	log.Debugf("Fingerprint config validation passed")
	return nil
}

// ComputeFingerprint computes the fingerprint for the given file path
func ComputeFingerprint(filePath string, fingerprintConfig *logsconfig.FingerprintConfig) uint64 {
	log.Debugf("Computing fingerprint for file: %s", filePath)

	fpFile, err := os.Open(filePath)
	if err != nil {
		log.Warnf("could not open file for fingerprinting %s: %v", filePath, err)
		return 0
	}
	defer fpFile.Close()

	if fingerprintConfig == nil {
		log.Warnf("fingerprint config is not set for file %q", filePath)
		return 0
	}

	linesSkipSet := fingerprintConfig.LinesToSkip != nil && *fingerprintConfig.LinesToSkip != 0
	bytesSkipSet := fingerprintConfig.BytesToSkip != nil && *fingerprintConfig.BytesToSkip != 0

	log.Debugf("Fingerprint mode for %s - linesSkipSet: %v, bytesSkipSet: %v", filePath, linesSkipSet, bytesSkipSet)

	// Explicitly check for an invalid configuration where both skip modes are specified.
	if linesSkipSet && bytesSkipSet {
		log.Warnf("Invalid configuration for fingerprinting file %q: both linesToSkip and bytesToSkip are set. Fingerprinting is disabled.", filePath)
		return 0
	}

	maxLines := 0
	if fingerprintConfig.MaxLines != nil {
		maxLines = *fingerprintConfig.MaxLines
	}

	// Mode selection:
	// - If bytesToSkip is set, it's byte-mode.
	// - If maxLines is 0, it implies byte-mode as line-mode is not viable.
	// - Otherwise, it's line-mode.
	if bytesSkipSet || maxLines == 0 {
		log.Debugf("Using byte-based fingerprinting for file %s", filePath)
		result := computeFingerPrintByBytes(fpFile, filePath, fingerprintConfig)
		log.Debugf("Byte-based fingerprint result for %s: 0x%x", filePath, result)
		return result
	}

	// Line-based fingerprinting mode
	log.Debugf("Using line-based fingerprinting for file %s", filePath)
	fingerprint := computeFingerPrintByLines(fpFile, filePath, fingerprintConfig)
	if fingerprint == 0 {
		log.Debugf("Not enough data for line-based fingerprinting of file %q", filePath)
	} else {
		log.Debugf("Line-based fingerprint result for %s: 0x%x", filePath, fingerprint)
	}
	return fingerprint
}

// computeFileFingerPrintByBytes computes fingerprint using byte-based approach for a given file path
func computeFingerPrintByBytes(fpFile *os.File, filePath string, fingerprintConfig *logsconfig.FingerprintConfig) uint64 {
	log.Debugf("Starting byte-based fingerprinting for file: %s", filePath)

	bytesToSkip := 0
	if fingerprintConfig.BytesToSkip != nil {
		bytesToSkip = *fingerprintConfig.BytesToSkip
	}
	log.Debugf("Skipping %d bytes for file %s", bytesToSkip, filePath)

	// Skip the configured number of bytes
	if bytesToSkip > 0 {
		_, err := io.CopyN(io.Discard, fpFile, int64(bytesToSkip))

		if err != nil && err != io.EOF {
			log.Warnf("Failed to skip %d bytes while computing fingerprint for %q: %v", bytesToSkip, filePath, err)
			return 0
		}
		log.Debugf("Successfully skipped %d bytes for file %s", bytesToSkip, filePath)
	}

	maxBytes := 0
	if fingerprintConfig.MaxBytes != nil {
		maxBytes = *fingerprintConfig.MaxBytes
	}
	log.Debugf("Reading up to %d bytes for fingerprinting file %s", maxBytes, filePath)

	// Create a limited reader for the bytes we want to hash
	limitedReader := &io.LimitedReader{R: fpFile, N: int64(maxBytes)}

	// Read up to maxBytes for hashing
	buffer := make([]byte, maxBytes)
	bytesRead, err := io.ReadFull(limitedReader, buffer)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		log.Warnf("Failed to read bytes for fingerprint %q: %v", filePath, err)
		return 0
	}

	log.Debugf("Read %d bytes for fingerprinting file %s (requested: %d)", bytesRead, filePath, maxBytes)

	// Trim buffer to actual bytes read
	buffer = buffer[:bytesRead]

	// Check if we have enough bytes to create a meaningful fingerprint
	if bytesRead == 0 || bytesRead < maxBytes {
		log.Debugf("No bytes available for fingerprinting file %q (read: %d, required: %d)", filePath, bytesRead, maxBytes)
		return 0
	}

	// Compute fingerprint
	log.Debugf("Buffer contents before checksum computation for file %q: %q", filePath, string(buffer))
	log.Debugf("Buffer hex dump for file %q: %x", filePath, buffer)
	log.Debugf("Buffer length: %d bytes", len(buffer))
	checksum := crc64.Checksum(buffer, crc64Table)

	log.Debugf("Computed byte-based fingerprint 0x%x for file %q (bytes=%d)", checksum, filePath, bytesRead)
	return checksum
}

// computeFileFingerPrintByLines computes fingerprint using line-based approach for a given file path
func computeFingerPrintByLines(fpFile *os.File, filePath string, fingerprintConfig *logsconfig.FingerprintConfig) uint64 {
	log.Debugf("Starting line-based fingerprinting for file: %s", filePath)
	reader := bufio.NewReader(fpFile)

	linesToSkip := 0
	if fingerprintConfig.LinesToSkip != nil {
		linesToSkip = *fingerprintConfig.LinesToSkip
	}
	log.Debugf("Skipping %d lines for file %s", linesToSkip, filePath)

	// Skip the configured number of lines
	for i := 0; i < linesToSkip; i++ {
		if _, err := reader.ReadString('\n'); err != nil {
			if err != io.EOF {
				log.Warnf("Failed to skip line while computing fingerprint for %q: %v", filePath, err)
			}
			log.Debugf("Failed to skip line %d for file %s", i+1, filePath)
			return 0
		}
		log.Debugf("Successfully skipped line %d for file %s", i+1, filePath)
	}

	// Read lines for hashing
	var buffer []byte
	linesRead := 0
	maxLines := 0
	if fingerprintConfig.MaxLines != nil {
		maxLines = *fingerprintConfig.MaxLines
	}
	maxBytes := 0
	if fingerprintConfig.MaxBytes != nil {
		maxBytes = *fingerprintConfig.MaxBytes
	}
	bytesRead := 0
	log.Debugf("Reading up to %d lines and %d bytes for fingerprinting file %s", maxLines, maxBytes, filePath)

	for linesRead < maxLines {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if len(line)+bytesRead > maxBytes {
				line = line[:maxBytes-bytesRead] //subtract the minimum number of bytes to make the line fit in maxBytes
				log.Debugf("Truncating line %d to fit in maxBytes limit", linesRead+1)
			}
			buffer = append(buffer, line...)
			linesRead++
			bytesRead += len(line)
			log.Debugf("Read line %d for file %s (bytes: %d, total bytes: %d)", linesRead, filePath, len(line), bytesRead)

			// If we've reached maxBytes, we have enough data for fingerprinting
			if bytesRead >= maxBytes {
				log.Debugf("Reached maxBytes limit (%d), stopping line reading", maxBytes)
				break
			}
		}

		if err != nil {
			if err != io.EOF {
				log.Warnf("Error while reading file for fingerprint %q: %v", filePath, err)
			}
			log.Debugf("Reached end of file after reading %d lines", linesRead)
			break
		}
	}

	// Check if we have enough lines to create a meaningful fingerprint
	if linesRead == 0 || (linesRead < maxLines && bytesRead < maxBytes) {
		log.Debugf("No lines available for fingerprinting file %q (linesRead: %d, maxLines: %d, bytesRead: %d, maxBytes: %d)",
			filePath, linesRead, maxLines, bytesRead, maxBytes)
		return 0
	}

	// Compute fingerprint
	log.Debugf("Buffer contents before checksum computation for file %q: %q", filePath, string(buffer))
	checksum := crc64.Checksum(buffer, crc64Table)
	log.Debugf("Computed line-based fingerprint 0x%x for file %q (bytes=%d, lines=%d)", checksum, filePath, len(buffer), linesRead)
	return checksum
}
