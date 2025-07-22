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

// FingerprintConfig is the configuration for the checksum fingerprinting algorithm
type FingerprintConfig struct {
	MaxBytes int `mapstructure:"max_bytes" json:"max_bytes" yaml:"max_bytes"`
	MaxLines int `mapstructure:"max_lines" json:"max_lines" yaml:"max_lines"`
	ToSkip   int `mapstructure:"to_skip" json:"to_skip" yaml:"to_skip"`
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

// ResolveFingerprintStrategy returns the fingerprint strategy for a given file.
// It checks the source-specific strategy first, then falls back to the global strategy.
func ResolveFingerprintStrategy(file *File) string {
	// Check if source has a specific fingerprint strategy set
	if file.Source.Config().FingerprintStrategy != "" {
		return file.Source.Config().FingerprintStrategy
	}

	// Fall back to global fingerprint strategy
	return pkgconfigsetup.Datadog().GetString("logs_config.fingerprint_strategy")
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

	maxLines := fingerprintConfig.MaxLines

	// Mode selection:
	// - If maxLines is 0, it implies byte-mode as line-mode is not viable.
	// - Otherwise, it's line-mode.
	if maxLines == 0 {
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
	bytesToSkip := fingerprintConfig.ToSkip
	// Skip the configured number of bytes
	if bytesToSkip > 0 {
		_, err := fpFile.Seek(int64(bytesToSkip), io.SeekStart)

		if err != nil && err != io.EOF {
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
	var limitedReader io.Reader = fpFile
	if maxBytes > 0 {
		limitedReader = io.LimitReader(fpFile, int64(maxBytes))
	}

	// Create scanner for line-by-line reading
	scanner := bufio.NewScanner(limitedReader)

	// Skip the configured number of lines
	for i := 0; i < linesToSkip; i++ {
		if !scanner.Scan() {
			if scanner.Err() != nil {
				log.Warnf("Failed to skip line while computing fingerprint for %q: %v", filePath, scanner.Err())
			}
			return 0
		}
	}

	// Read lines for hashing
	var buffer []byte
	linesRead := 0
	bytesRead := 0

	for linesRead < maxLines && scanner.Scan() {
		line := scanner.Text()
		lineWithNewline := line + "\n" // Add newline back for consistency

		// Check if adding this line would exceed maxBytes
		if maxBytes > 0 && bytesRead+len(lineWithNewline) > maxBytes {
			// Truncate the line to fit within maxBytes
			remainingBytes := maxBytes - bytesRead
			if remainingBytes > 0 {
				lineWithNewline = lineWithNewline[:remainingBytes]
				log.Debugf("Truncated line to fit within %d bytes for fingerprinting", maxBytes)
			} else {
				break // No more space for additional lines
			}
		}

		buffer = append(buffer, []byte(lineWithNewline)...)
		linesRead++
		bytesRead += len(lineWithNewline)
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		log.Warnf("Error while reading file for fingerprint %q: %v", filePath, err)
		return 0
	}

	// Check if we have enough lines to create a meaningful fingerprint
	if linesRead == 0 || (maxLines > 0 && linesRead < maxLines && maxBytes > 0 && bytesRead < maxBytes) {
		log.Debugf("No lines available for fingerprinting file %q", filePath)
		return 0
	}

	// Compute fingerprint
	checksum := crc64.Checksum(buffer, crc64Table)
	log.Debugf("Computed line-based fingerprint 0x%x for file %q (bytes=%d, lines=%d)", checksum, filePath, len(buffer), linesRead)
	return checksum
}
