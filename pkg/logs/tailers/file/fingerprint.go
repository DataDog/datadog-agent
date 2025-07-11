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

	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ReturnFingerprintConfig returns the configuration for the fingerprinting algorithm set by user (also used for testing)
func ReturnFingerprintConfig() *logsconfig.FingerprintConfig {
	if pkgconfigsetup.Datadog().GetString("logs_config.fingerprint_strategy") != "checksum" {
		return nil
	}
	maxLines := pkgconfigsetup.Datadog().GetInt("logs_config.fingerprint_config.max_lines")
	maxBytes := pkgconfigsetup.Datadog().GetInt("logs_config.fingerprint_config.max_bytes")
	bytesToSkip := pkgconfigsetup.Datadog().GetInt("logs_config.fingerprint_config.bytes_to_skip")
	linesToSkip := pkgconfigsetup.Datadog().GetInt("logs_config.fingerprint_config.lines_to_skip")
	return &logsconfig.FingerprintConfig{
		MaxLines:    &maxLines,
		MaxBytes:    &maxBytes,
		BytesToSkip: &bytesToSkip,
		LinesToSkip: &linesToSkip,
	}
}

// ComputeFingerprint computes the fingerprint for the given file path
func ComputeFingerprint(filePath string, fingerprintConfig *logsconfig.FingerprintConfig) uint64 {
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
		return computeFingerPrintByBytes(fpFile, filePath, fingerprintConfig)
	}

	// Line-based fingerprinting mode
	fingerprint := computeFingerPrintByLines(fpFile, filePath, fingerprintConfig)
	if fingerprint == 0 {
		log.Debugf("Not enough data for line-based fingerprinting of file %q", filePath)
	}
	return fingerprint
}

// computeFileFingerPrintByBytes computes fingerprint using byte-based approach for a given file path
func computeFingerPrintByBytes(fpFile *os.File, filePath string, fingerprintConfig *logsconfig.FingerprintConfig) uint64 {
	bytesToSkip := 0
	if fingerprintConfig.BytesToSkip != nil {
		bytesToSkip = *fingerprintConfig.BytesToSkip
	}
	// Skip the configured number of bytes
	if bytesToSkip > 0 {
		_, err := io.CopyN(io.Discard, fpFile, int64(bytesToSkip))

		if err != nil && err != io.EOF {
			log.Warnf("Failed to skip %d bytes while computing fingerprint for %q: %v", bytesToSkip, filePath, err)
			return 0
		}
	}

	maxBytes := 0
	if fingerprintConfig.MaxBytes != nil {
		maxBytes = *fingerprintConfig.MaxBytes
	}

	// Create a limited reader for the bytes we want to hash
	limitedReader := &io.LimitedReader{R: fpFile, N: int64(maxBytes)}

	// Read up to maxBytes for hashing
	buffer := make([]byte, maxBytes)
	bytesRead, err := io.ReadFull(limitedReader, buffer)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		log.Warnf("Failed to read bytes for fingerprint %q: %v", filePath, err)
		return 0
	}

	// Trim buffer to actual bytes read
	buffer = buffer[:bytesRead]

	// Check if we have enough bytes to create a meaningful fingerprint
	if bytesRead == 0 || bytesRead < maxBytes {
		log.Debugf("No bytes available for fingerprinting file %q", filePath)
		return 0
	}

	// Compute fingerprint
	table := crc64.MakeTable(crc64.ISO)
	checksum := crc64.Checksum(buffer, table)

	log.Debugf("Computed byte-based fingerprint 0x%x for file %q (bytes=%d)", checksum, filePath, bytesRead)
	return checksum
}

// computeFileFingerPrintByLines computes fingerprint using line-based approach for a given file path
func computeFingerPrintByLines(fpFile *os.File, filePath string, fingerprintConfig *logsconfig.FingerprintConfig) uint64 {
	reader := bufio.NewReader(fpFile)

	linesToSkip := 0
	if fingerprintConfig.LinesToSkip != nil {
		linesToSkip = *fingerprintConfig.LinesToSkip
	}

	// Skip the configured number of lines
	for i := 0; i < linesToSkip; i++ {
		if _, err := reader.ReadString('\n'); err != nil {
			if err != io.EOF {
				log.Warnf("Failed to skip line while computing fingerprint for %q: %v", filePath, err)
			}
			return 0
		}
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
	for linesRead < maxLines {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			// Cap the line at maxBytes bytes
			if len(line) > maxBytes {
				line = line[:maxBytes]
				log.Debugf("Truncated line from original length to %d bytes for fingerprinting", maxBytes)
			} else if len(line)+bytesRead > maxBytes {
				line = line[:maxBytes-bytesRead] //subtract the minimum number of bytes to make the line fit in maxBytes
			}
			buffer = append(buffer, line...)
			linesRead++
			bytesRead += len(line)

			// If we've reached maxBytes, we have enough data for fingerprinting
			if bytesRead >= maxBytes {
				break
			}
		}

		if err != nil {
			if err != io.EOF {
				log.Warnf("Error while reading file for fingerprint %q: %v", filePath, err)
			}
			break
		}
	}

	// Check if we have enough lines to create a meaningful fingerprint
	if linesRead == 0 || (linesRead < maxLines && bytesRead < maxBytes) {
		log.Debugf("No lines available for fingerprinting file %q", filePath)
		return 0
	}

	// Compute fingerprint
	table := crc64.MakeTable(crc64.ISO)
	checksum := crc64.Checksum(buffer, table)
	log.Debugf("Computed line-based fingerprint 0x%x for file %q (bytes=%d, lines=%d)", checksum, filePath, len(buffer), linesRead)
	return checksum
}
