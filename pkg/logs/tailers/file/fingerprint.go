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

// FingerprintConfig is the configuration for the checksum fingerprinting algorithm
type FingerprintConfig struct {
	MaxBytes int `mapstructure:"max_bytes" json:"max_bytes" yaml:"max_bytes"`
	MaxLines int `mapstructure:"max_lines" json:"max_lines" yaml:"max_lines"`
	ToSkip   int `mapstructure:"to_skip" json:"to_skip" yaml:"to_skip"`
	// FingerprintStrategy defines the strategy used for fingerprinting. Options are:
	// - "line_checksum": compute checksum based on line content (default)
	// - "byte_checksum": compute checksum based on byte content
	FingerprintStrategy string `mapstructure:"fingerprint_strategy" json:"fingerprint_strategy" yaml:"fingerprint_strategy"`
}

// crc64Table is a package-level variable for the CRC64 ISO table
// to avoid recreating it on every fingerprint computation
var crc64Table = crc64.MakeTable(crc64.ISO)

// ReturnFingerprintConfig returns the configuration for the fingerprinting algorithm set by user (also used for testing)
func ReturnFingerprintConfig(sourceConfig *logsconfig.FingerprintConfig, sourceStrategy string) *logsconfig.FingerprintConfig {
	// If per-source config is set and strategy is "checksum", use it
	if sourceStrategy == "checksum" && sourceConfig != nil {
		return sourceConfig
	}

	// Otherwise, use the global config with proper unmarshalling
	globalConfig, err := logsconfig.GlobalFingerprintConfig(pkgconfigsetup.Datadog())
	if err != nil {
		log.Warnf("Failed to load global fingerprint config: %v", err)
		return nil
	}
	return globalConfig
}

// ResolveRotationDetectionStrategy returns the rotation detection strategy for a given file.
// It checks the source-specific strategy first, then falls back to the global strategy.
func ResolveRotationDetectionStrategy(file *File) string {
	// Check if source has a specific rotation detection strategy set
	if file.Source.Config().RotationDetectionStrategy != "" {
		return file.Source.Config().RotationDetectionStrategy
	}

	// Fall back to global rotation detection strategy
	return pkgconfigsetup.Datadog().GetString("logs_config.rotation_detection_strategy")
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

	// Determine fingerprinting strategy
	strategy := fingerprintConfig.FingerprintStrategy
	if strategy == "" {
		// Default to line_checksum if no strategy is specified
		strategy = "line_checksum"
	}

	// Mode selection based on strategy:
	// - "byte_checksum": use byte-based fingerprinting
	// - "line_checksum" or default: use line-based fingerprinting
	if strategy == "byte_checksum" {
		return computeFingerPrintByBytes(fpFile, filePath, fingerprintConfig)
	}

	// Line-based fingerprinting mode (default)
	fingerprint := computeFingerPrintByLines(fpFile, filePath, fingerprintConfig)
	if fingerprint == 0 {
		log.Debugf("Not enough data for line-based fingerprinting of file %q", filePath)
	}
	return fingerprint
}

// computeFileFingerPrintByBytes computes fingerprint using byte-based approach for a given file path
func computeFingerPrintByBytes(fpFile *os.File, filePath string, fingerprintConfig *logsconfig.FingerprintConfig) uint64 {
	bytesToSkip := fingerprintConfig.ToSkip
	// Skip the configured number of bytes
	if bytesToSkip > 0 {
		_, err := fpFile.Seek(int64(bytesToSkip), io.SeekStart)

		if err != nil {
			log.Warnf("Failed to skip %d bytes while computing fingerprint for %q: %v", bytesToSkip, filePath, err)
			return 0
		}
	}

	maxBytes := fingerprintConfig.MaxBytes

	// Read up to maxBytes for hashing
	buffer := make([]byte, maxBytes)
	bytesRead, err := io.ReadFull(fpFile, buffer)
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
	checksum := crc64.Checksum(buffer, crc64Table)

	log.Debugf("Computed byte-based fingerprint 0x%x for file %q (bytes=%d)", checksum, filePath, bytesRead)
	return checksum
}

// computeFileFingerPrintByLines computes fingerprint using line-based approach for a given file path
func computeFingerPrintByLines(fpFile *os.File, filePath string, fingerprintConfig *logsconfig.FingerprintConfig) uint64 {
	linesToSkip := fingerprintConfig.ToSkip
	maxLines := fingerprintConfig.MaxLines
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
			// If we hit EOF during skip and limitedReader.N is 0, fall back to bytes fingerprint
			if lr, ok := limitedReader.(*io.LimitedReader); ok && lr.N == 0 {
				log.Debugf("Reached maxBytes during line skip, falling back to bytes fingerprint for %q", filePath)
				return computeFingerPrintByBytes(fpFile, filePath, fingerprintConfig)
			}
			return 0
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
		return 0
	}

	fmt.Println(bytesRead < maxBytes)
	fmt.Println(bytesRead)
	// Check if we have enough lines to create a meaningful fingerprint
	if linesRead == 0 || (linesRead < maxLines && bytesRead < maxBytes) {
		log.Debugf("Not enough data for fingerprinting file %q (lines=%d, bytes=%d)", filePath, linesRead, bytesRead)
		return 0
	}

	// Compute fingerprint
	fmt.Println("This is the content in side the buffer: ", string(buffer))
	checksum := crc64.Checksum(buffer, crc64Table)
	log.Debugf("Computed line-based fingerprint 0x%x for file %q (bytes=%d, lines=%d)", checksum, filePath, len(buffer), linesRead)
	return checksum
}
