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
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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

// Fingerprinter is a struct that contains the fingerprinting configuration
type Fingerprinter struct {
	FingerprintConfig types.FingerprintConfig
}

// NewFingerprinter creates a new Fingerprinter with the given configuration
func NewFingerprinter(fingerprintConfig types.FingerprintConfig) *Fingerprinter {
	log.Debugf("Creating new fingerprinter: config=%+v", fingerprintConfig)
	return &Fingerprinter{
		FingerprintConfig: fingerprintConfig,
	}
}

// newInvalidFingerprint returns a fingerprint with Value=0 to represent an invalid/empty fingerprint
func newInvalidFingerprint(config *types.FingerprintConfig) *types.Fingerprint {
	return &types.Fingerprint{Value: types.InvalidFingerprintValue, Config: config}
}

// crc64Table is a package-level variable for the CRC64 ISO table
// to avoid recreating it on every fingerprint computation
var crc64Table = crc64.MakeTable(crc64.ISO)

// ShouldFileFingerprint returns whether or not a given file should be fingerprinted to detect rotation and truncation
func (f *Fingerprinter) ShouldFileFingerprint(file *File) bool {
	fileFingerprintConfig := file.Source.Config().FingerprintConfig

	// Check per-source config first (takes precedence over global config)
	if fileFingerprintConfig != nil {
		if fileFingerprintConfig.FingerprintStrategy == types.FingerprintStrategyDisabled {
			log.Debugf("Fingerprinting disabled for source %s, skipping file %s", file.Source.Config().Source, file.Path)
			return false
		}
		if fileFingerprintConfig.FingerprintStrategy != "" {
			log.Debugf("File %s will be fingerprinted with per-source config (strategy: %s)", file.Path, fileFingerprintConfig.FingerprintStrategy)
			return true
		}
	}

	// Now, check global config
	globalConfig := f.GetFingerprintConfig()
	if globalConfig == nil {
		log.Debugf("No fingerprint config available for file %s (source config: %+v, global config: %+v)",
			file.Path,
			file.Source.Config().FingerprintConfig,
			f.GetFingerprintConfig())
		return false
	}

	if globalConfig.FingerprintStrategy == types.FingerprintStrategyDisabled {
		log.Debugf("Fingerprinting disabled for file %s based on global config, skipping file %s", file.Path, file.Path)
		return false
	}

	log.Debugf("File %s will be fingerprinted with global config: %s", file.Path, globalConfig.FingerprintStrategy)
	return true
}

// ComputeFingerprintFromConfig computes the fingerprint for the given file path using a specific config
func (f *Fingerprinter) ComputeFingerprintFromConfig(filepath string, fingerprintConfig *types.FingerprintConfig) (*types.Fingerprint, error) {
	if fingerprintConfig != nil && fingerprintConfig.FingerprintStrategy == types.FingerprintStrategyDisabled {
		return newInvalidFingerprint(nil), nil
	}
	return computeFingerprint(filepath, fingerprintConfig)
}

// ComputeFingerprint computes the fingerprint for the given file path
func (f *Fingerprinter) ComputeFingerprint(file *File) (*types.Fingerprint, error) {
	if file == nil {
		log.Warnf("file is nil, skipping fingerprinting")
		return newInvalidFingerprint(nil), nil
	}

	fileFingerprintConfig := file.Source.Config().FingerprintConfig

	// Check per-source config first (takes precedence over global config)
	if fileFingerprintConfig != nil && fileFingerprintConfig.FingerprintStrategy != "" {
		if fileFingerprintConfig.FingerprintStrategy == types.FingerprintStrategyDisabled {
			log.Debugf("Fingerprinting disabled for source %s, returning invalid fingerprint", file.Source.Config().Source)
			return newInvalidFingerprint(nil), nil
		}

		// Convert from config.FingerprintConfig to types.FingerprintConfig
		fingerprintConfig := &types.FingerprintConfig{
			FingerprintStrategy: types.FingerprintStrategy(fileFingerprintConfig.FingerprintStrategy),
			Count:               fileFingerprintConfig.Count,
			CountToSkip:         fileFingerprintConfig.CountToSkip,
			MaxBytes:            fileFingerprintConfig.MaxBytes,
		}

		log.Debugf("Using file source fingerprint config: strategy=%s, count=%d, countToSkip=%d, maxBytes=%d",
			fingerprintConfig.FingerprintStrategy,
			fingerprintConfig.Count,
			fingerprintConfig.CountToSkip,
			fingerprintConfig.MaxBytes)

		return computeFingerprint(file.Path, fingerprintConfig)
	}

	// If per-source config exists but no strategy is set, or no per-source config exists,
	// fall back to global config
	fingerprintConfig := f.GetFingerprintConfig()
	if fingerprintConfig == nil {
		log.Debugf("No fingerprint config found in file source or defaults, returning invalid fingerprint")
		return newInvalidFingerprint(nil), nil
	}

	// Use the global config directly since it's already the right type
	log.Debugf("Using global fingerprint config: strategy=%s, count=%d, countToSkip=%d, maxBytes=%d",
		fingerprintConfig.FingerprintStrategy,
		fingerprintConfig.Count,
		fingerprintConfig.CountToSkip,
		fingerprintConfig.MaxBytes)
	return computeFingerprint(file.Path, fingerprintConfig)
}

// computeFingerprint computes the fingerprint for the given file path
func computeFingerprint(filePath string, fingerprintConfig *types.FingerprintConfig) (*types.Fingerprint, error) {
	if fingerprintConfig == nil {
		log.Debugf("No fingerprint config provided, returning invalid fingerprint")
		return newInvalidFingerprint(nil), nil
	}

	log.Debugf("Computing fingerprint for %s with config: strategy=%s, count=%d, countToSkip=%d, maxBytes=%d",
		filePath,
		fingerprintConfig.FingerprintStrategy,
		fingerprintConfig.Count,
		fingerprintConfig.CountToSkip,
		fingerprintConfig.MaxBytes)

	fpFile, err := filesystem.OpenShared(filePath)
	if err != nil {
		log.Warnf("could not open file for fingerprinting %s: %v", filePath, err)
		return newInvalidFingerprint(nil), err
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
func computeFingerPrintByBytes(fpFile *os.File, filePath string, fingerprintConfig *types.FingerprintConfig) (*types.Fingerprint, error) {
	bytesToSkip := fingerprintConfig.CountToSkip
	maxBytes := fingerprintConfig.Count
	// Skip the configured number of bytes
	if bytesToSkip > 0 {
		_, err := fpFile.Seek(int64(bytesToSkip), io.SeekStart)

		if err != nil {
			log.Warnf("Failed to skip %d bytes while computing fingerprint for %q: %v", bytesToSkip, filePath, err)
			return newInvalidFingerprint(fingerprintConfig), err
		}
	}

	// Read up to maxBytes for hashing
	buffer := make([]byte, maxBytes)
	bytesRead, err := io.ReadFull(fpFile, buffer)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		log.Warnf("Failed to read bytes for fingerprint %q: %v", filePath, err)
		return newInvalidFingerprint(fingerprintConfig), err
	}

	// Check if we have enough bytes to create a meaningful fingerprint
	if bytesRead == 0 || bytesRead < maxBytes {
		log.Debugf("No bytes available for fingerprinting file %q", filePath)
		return newInvalidFingerprint(fingerprintConfig), nil
	}

	// Compute fingerprint
	checksum := crc64.Checksum(buffer, crc64Table)

	log.Debugf("Computed byte-based fingerprint 0x%x for file %q (bytes=%d)", checksum, filePath, bytesRead)
	return &types.Fingerprint{Value: checksum, Config: fingerprintConfig}, nil
}

// computeFingerPrintByLines computes fingerprint using line-based approach for a given file path
func computeFingerPrintByLines(fpFile *os.File, filePath string, fingerprintConfig *types.FingerprintConfig) (*types.Fingerprint, error) {
	linesToSkip := fingerprintConfig.CountToSkip
	maxLines := fingerprintConfig.Count
	maxBytes := fingerprintConfig.MaxBytes

	// Create a LimitedReader to respect maxBytes constraint
	limitedReader := io.LimitReader(fpFile, int64(maxBytes))

	// Create scanner for line-by-line reading
	scanner := bufio.NewScanner(limitedReader)

	// Single loop that handles both skipping and reading
	var buffer []byte
	linesRead := 0

	for i := 0; i < linesToSkip+maxLines; i++ {
		if scanner.Scan() {
			if i >= linesToSkip {
				line := scanner.Bytes()
				buffer = append(buffer, line...)
				linesRead++
			}
		} else {
			/// Check if we need to fall back due to byte limits
			if limitedReader.(*io.LimitedReader).N == 0 {
				log.Warnf("Scanner stopped with no bytes remaining, falling back to byte-based fingerprint for %q", filePath)
				pos, err := fpFile.Seek(0, io.SeekStart)
				if pos != 0 || err != nil {
					log.Warnf("Error %s occurred while trying to reset file offset", err)
					return newInvalidFingerprint(fingerprintConfig), err
				}
				return computeFingerPrintByBytes(fpFile, filePath, defaultBytesConfig)
			}
			// Handle scanner errors
			if err := scanner.Err(); err != nil {
				log.Warnf("Error while reading file for fingerprint %q: %v", filePath, err)
				return newInvalidFingerprint(fingerprintConfig), err
			}
			// Check if we have enough data for fingerprinting
			// We need either enough lines OR enough bytes to create a meaningful fingerprint
			log.Debugf("Not enough data for fingerprinting file %q", filePath)
			return newInvalidFingerprint(fingerprintConfig), nil

		}
	}

	// Compute fingerprint
	checksum := crc64.Checksum(buffer, crc64Table)
	log.Debugf("Computed line-based fingerprint 0x%x for file %q (bytes=%d, lines=%d)", checksum, filePath, len(buffer), linesRead)
	return &types.Fingerprint{Value: checksum, Config: fingerprintConfig}, nil
}

// GetFingerprintConfig returns the fingerprint configuration
func (f *Fingerprinter) GetFingerprintConfig() *types.FingerprintConfig {
	return &f.FingerprintConfig
}
