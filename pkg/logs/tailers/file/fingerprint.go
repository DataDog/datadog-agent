// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"hash/crc64"
	"io"
	"runtime"
	"slices"
	"strings"

	"github.com/spf13/afero"

	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Fallback fingerprint configs used when requested fingerprint
// strategy (bytes or line-based) can not be used.
var defaultBytesConfig = &types.FingerprintConfig{
	FingerprintStrategy: types.FingerprintStrategyByteChecksum,
	Count:               1024,
	CountToSkip:         0,
	Source:              types.FingerprintConfigSourceDefault,
}

// DefaultLinesConfig provides a sensible default configuration for line-based fingerprinting
var defaultLinesConfig = &types.FingerprintConfig{
	FingerprintStrategy: types.FingerprintStrategyLineChecksum,
	Count:               1,
	CountToSkip:         0,
	MaxBytes:            10000,
	Source:              types.FingerprintConfigSourceDefault,
}

// Fingerprinter is an interface that defines the methods for fingerprinting files
type Fingerprinter interface {
	// ShouldFileFingerprint returns whether or not a given file should be fingerprinted to detect rotation and truncation
	ShouldFileFingerprint(file *File) bool
	// ComputeFingerprint computes the fingerprint for the given file path
	ComputeFingerprint(file *File) (*types.Fingerprint, error)
	// ComputeFingerprintFromHandle computes the fingerprint for the given os.File using the provided config
	ComputeFingerprintFromHandle(osFile afero.File, fingerprintConfig *types.FingerprintConfig) (*types.Fingerprint, error)
	// ComputeFingerprintFromConfig computes the fingerprint for the given file path using a specific config
	ComputeFingerprintFromConfig(filepath string, fingerprintConfig *types.FingerprintConfig) (*types.Fingerprint, error)
	// GetEffectiveConfigForFile returns the fingerprint configuration that applies to a file,
	// including disabled configurations used for status display.
	GetEffectiveConfigForFile(file *File) *types.FingerprintConfig
}

// fingerprinterImpl is a struct that contains the fingerprinting configuration
type fingerprinterImpl struct {
	globalConfig types.FingerprintConfig
	fileOpener   opener.FileOpener
}

// FingerprintConfigInfo holds fingerprint configuration for status display
type FingerprintConfigInfo struct {
	config *types.FingerprintConfig
}

// NewFingerprinter creates a new Fingerprinter with the given configuration
func NewFingerprinter(fingerprintConfig types.FingerprintConfig, opener opener.FileOpener) Fingerprinter {
	return &fingerprinterImpl{
		globalConfig: fingerprintConfig,
		fileOpener:   opener,
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
//
// Precedence lives in GetEffectiveConfigForFile so this cannot disagree with the config used.
func (f *fingerprinterImpl) ShouldFileFingerprint(file *File) bool {
	effectiveConfig := f.GetEffectiveConfigForFile(file)
	return effectiveConfig != nil && effectiveConfig.FingerprintStrategy != types.FingerprintStrategyDisabled
}

// ComputeFingerprintFromConfig computes the fingerprint for the given file path using a specific config
// Note that the provided configuration can fallback to different default configuration if specific errors occur attempting to compute the fingerprint.
func (f *fingerprinterImpl) ComputeFingerprintFromConfig(filepath string, fingerprintConfig *types.FingerprintConfig) (*types.Fingerprint, error) {
	if fingerprintConfig != nil && fingerprintConfig.FingerprintStrategy == types.FingerprintStrategyDisabled {
		return newInvalidFingerprint(fingerprintConfig), nil
	}
	return f.computeFingerprint(filepath, fingerprintConfig)
}

// ComputeFingerprint computes the fingerprint for the given file path
// The fingerprint configuration is automatically derived from the file source configuration if present,
// otherwise the global config is preferred.
// Note that the provided configuration can fallback to different default configuration if specific errors occur attempting to compute the fingerprint.
func (f *fingerprinterImpl) ComputeFingerprint(file *File) (*types.Fingerprint, error) {
	if file == nil {
		log.Warnf("file is nil, skipping fingerprinting")
		return newInvalidFingerprint(nil), nil
	}

	return f.ComputeFingerprintFromConfig(file.Path, f.GetEffectiveConfigForFile(file))
}

// ComputeFingerprintFromHandle computes the fingerprint for the given os.File using the provided config.
// Note that the providedconfiguration can fallback to different default configuration if specific errors occur attempting to compute the fingerprint.
func (f *fingerprinterImpl) ComputeFingerprintFromHandle(osFile afero.File, fingerprintConfig *types.FingerprintConfig) (*types.Fingerprint, error) {
	if fingerprintConfig == nil {
		return newInvalidFingerprint(nil), nil
	}
	if osFile == nil {
		return newInvalidFingerprint(nil), errors.New("osFile cannot be nil")
	}
	return f.computeFingerprintFromReader(osFile, osFile.Name(), fingerprintConfig)
}

// computeFingerprintFromReader hashes the bytes reachable from reader, which is
// either a buffered descriptor or a reader over a descriptor opened with
// open_flags.
func (f *fingerprinterImpl) computeFingerprintFromReader(reader io.ReadSeeker, filePath string, fingerprintConfig *types.FingerprintConfig) (*types.Fingerprint, error) {
	// Determine fingerprinting strategy (line_checksum or byte_checksum)
	strategy := fingerprintConfig.FingerprintStrategy
	switch strategy {
	case types.FingerprintStrategyLineChecksum:
		return computeFingerPrintByLines(reader, filePath, fingerprintConfig)
	case types.FingerprintStrategyByteChecksum:
		return computeFingerPrintByBytes(reader, filePath, fingerprintConfig)
	default:
		log.Warnf("invalid fingerprint strategy %q for file %q, using default lines strategy", strategy, filePath)
		// Default to line_checksum if no strategy is specified
		return computeFingerPrintByLines(reader, filePath, defaultLinesConfig)
	}
}

// computeFingerprint fingerprints filePath with the configured open_flags.
//
// When those flags turn out to be unusable for this file the error is returned
// rather than retried without them. Silently reading through a different path
// than the one that was asked for leaves the Agent reporting a configuration it
// is not honouring, so an unusable open_flags setting is treated as a
// misconfiguration for the operator to correct. The launcher reports it per
// file on the status page.
func (f *fingerprinterImpl) computeFingerprint(filePath string, fingerprintConfig *types.FingerprintConfig) (*types.Fingerprint, error) {
	if fingerprintConfig == nil {
		return newInvalidFingerprint(nil), nil
	}

	openFlags := fingerprintConfig.OpenFlags
	if fingerprintOpenFlagsActive(openFlags) {
		return f.computeFingerprintDirect(filePath, fingerprintConfig)
	}

	fpFile, err := f.fileOpener.OpenLogFile(filePath)
	if err != nil {
		log.Warnf("could not open file for fingerprinting %s: %v", filePath, err)
		return newInvalidFingerprint(fingerprintConfig), err
	}
	defer fpFile.Close()

	return f.computeFingerprintFromReader(fpFile, filePath, fingerprintConfig)
}

func (f *fingerprinterImpl) computeFingerprintDirect(filePath string, fingerprintConfig *types.FingerprintConfig) (*types.Fingerprint, error) {
	openFlags := fingerprintConfig.OpenFlags
	switch fingerprintConfig.FingerprintStrategy {
	case types.FingerprintStrategyByteChecksum:
		data, err := f.fileOpener.ReadDirectFingerprintRange(filePath, fingerprintConfig.CountToSkip, fingerprintConfig.Count, openFlags)
		if err != nil {
			return newInvalidFingerprint(fingerprintConfig), err
		}
		return fingerprintFromByteData(data, fingerprintConfig.Count, fingerprintConfig)
	case types.FingerprintStrategyLineChecksum:
		return f.computeFingerprintByLinesDirect(filePath, openFlags, fingerprintConfig)
	default:
		log.Warnf("invalid fingerprint strategy %q for file %q, using default lines strategy", fingerprintConfig.FingerprintStrategy, filePath)
		return f.computeFingerprintByLinesDirect(filePath, openFlags, defaultLinesConfig)
	}
}

// computeFingerprintByLinesDirect reads the bounded head of the file once with
// the configured open_flags, then runs the shared buffered line-fingerprint flow
// over the in-memory bytes. This keeps the direct and buffered paths on a single
// implementation so their checksums cannot drift.
//
// The read window covers both the line budget (MaxBytes) and the byte-fallback
// budget (defaultBytesConfig.Count). The line flow falls back to a byte checksum
// over the first bytes of the same reader when a line exceeds MaxBytes, so the
// window must hold enough bytes for that fallback without a second open.
func (f *fingerprinterImpl) computeFingerprintByLinesDirect(filePath string, openFlags []types.FileOpenFlag, hashConfig *types.FingerprintConfig) (*types.Fingerprint, error) {
	readBudget := max(hashConfig.MaxBytes, defaultBytesConfig.Count)
	data, err := f.fileOpener.ReadDirectFingerprintRange(filePath, 0, readBudget, openFlags)
	if err != nil {
		return newInvalidFingerprint(hashConfig), err
	}
	return computeFingerPrintByLines(bytes.NewReader(data), filePath, hashConfig)
}

// FingerprintOpenFlagsActive reports whether configured open_flags should be
// applied for this fingerprint read. They are Linux-only; other platforms use
// OpenLogFile and ignore the configured flags.
func FingerprintOpenFlagsActive(cfg *types.FingerprintConfig) bool {
	if cfg == nil {
		return false
	}
	return fingerprintOpenFlagsActive(cfg.OpenFlags)
}

// fingerprintOpenFlagsActive reports whether configured open_flags should be
// applied for this fingerprint read. They are Linux-only; other platforms use
// OpenLogFile and ignore the configured flags.
func fingerprintOpenFlagsActive(openFlags []types.FileOpenFlag) bool {
	return len(openFlags) > 0 && runtime.GOOS == "linux"
}

// computeFingerPrintByBytes computes fingerprint using byte-based approach for a given file path
func computeFingerPrintByBytes(fpFile io.ReadSeeker, filePath string, fingerprintConfig *types.FingerprintConfig) (*types.Fingerprint, error) {
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
		if !fingerprintOpenFlagsActive(fingerprintConfig.OpenFlags) {
			log.Warnf("Failed to read bytes for fingerprint %q: %v", filePath, err)
		}
		return newInvalidFingerprint(fingerprintConfig), err
	}

	return fingerprintFromByteData(buffer[:bytesRead], maxBytes, fingerprintConfig)
}

func fingerprintFromByteData(data []byte, wantBytes int, fingerprintConfig *types.FingerprintConfig) (*types.Fingerprint, error) {
	if len(data) == 0 || len(data) < wantBytes {
		return newInvalidFingerprint(fingerprintConfig), nil
	}

	checksum := crc64.Checksum(data[:wantBytes], crc64Table)
	return &types.Fingerprint{Value: checksum, Config: fingerprintConfig}, nil
}

// computeFingerPrintByLines computes fingerprint using line-based approach for a given file path
func computeFingerPrintByLines(fpFile io.ReadSeeker, filePath string, fingerprintConfig *types.FingerprintConfig) (*types.Fingerprint, error) {
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
				log.Warnf(
					"Ran out of space reading requested line count for fingerprinting, falling back to byte-based fingerprint for %q. "+
						"This is almost certainly indicative of a configuration error, please verify your fingerprint configuration.",
					filePath,
				)
				pos, err := fpFile.Seek(0, io.SeekStart)
				if pos != 0 || err != nil {
					log.Warnf("Error %s occurred while trying to reset file offset", err)
					return newInvalidFingerprint(fingerprintConfig), err
				}
				fallbackConfig := *defaultBytesConfig
				fallbackConfig.OpenFlags = append([]types.FileOpenFlag(nil), fingerprintConfig.OpenFlags...)
				return computeFingerPrintByBytes(fpFile, filePath, &fallbackConfig)
			}
			// Handle scanner errors
			if err := scanner.Err(); err != nil {
				if !fingerprintOpenFlagsActive(fingerprintConfig.OpenFlags) {
					log.Warnf("Error while reading file for fingerprint %q: %v", filePath, err)
				}
				return newInvalidFingerprint(fingerprintConfig), err
			}
			// Check if we have enough data for fingerprinting
			// We need either enough lines OR enough bytes to create a meaningful fingerprint
			return newInvalidFingerprint(fingerprintConfig), nil

		}
	}

	// Compute fingerprint
	checksum := crc64.Checksum(buffer, crc64Table)
	return &types.Fingerprint{Value: checksum, Config: fingerprintConfig}, nil
}

// GetEffectiveConfigForFile returns the fingerprint configuration that applies
// to a file, including disabled configurations used for status display.
func (f *fingerprinterImpl) GetEffectiveConfigForFile(file *File) *types.FingerprintConfig {
	if file == nil {
		return nil
	}

	fileFingerprintConfig := file.Source.Config().FingerprintConfig

	// Check per-source config first (takes precedence over global config)
	if fileFingerprintConfig != nil && fileFingerprintConfig.FingerprintStrategy != "" {
		// Copying the struct carries every parameter, including ones added later. OpenFlags is
		// cloned because the result outlives the call while the source can be replaced.
		perSourceConfig := *fileFingerprintConfig
		perSourceConfig.OpenFlags = slices.Clone(fileFingerprintConfig.OpenFlags)
		perSourceConfig.Source = types.FingerprintConfigSourcePerSource
		return &perSourceConfig
	}

	// Fall back to global config
	return &f.globalConfig
}

// InfoKey returns the key for this info
// This data is exposed in the status table
func (f *FingerprintConfigInfo) InfoKey() string {
	return "Fingerprint Config"
}

// Info returns formatted fingerprint configuration information
func (f *FingerprintConfigInfo) Info() []string {
	if f.config == nil {
		return []string{
			"Source: none",
			"Strategy: not configured",
		}
	}

	source := "none"
	if f.config.Source != "" {
		source = string(f.config.Source)
	}

	if f.config.FingerprintStrategy == types.FingerprintStrategyDisabled {
		return []string{
			"Source: " + source,
			"Strategy: disabled",
		}
	}

	info := []string{
		"Source: " + source,
		fmt.Sprintf("Strategy: %s", f.config.FingerprintStrategy),
	}

	// Add Count and CountToSkip for all strategies except disabled
	info = append(info,
		fmt.Sprintf("Count: %d", f.config.Count),
		fmt.Sprintf("CountToSkip: %d", f.config.CountToSkip),
	)

	if f.config.FingerprintStrategy == types.FingerprintStrategyLineChecksum {
		info = append(info, fmt.Sprintf("MaxBytes: %d", f.config.MaxBytes))
	}
	if len(f.config.OpenFlags) > 0 {
		flags := make([]string, len(f.config.OpenFlags))
		for i, openFlag := range f.config.OpenFlags {
			flags[i] = string(openFlag)
		}
		info = append(info, "OpenFlags: "+strings.Join(flags, ","))
	}

	return info
}

// NewFingerprintConfigInfo creates a new FingerprintConfigInfo from a FingerprintConfig
func NewFingerprintConfigInfo(config *types.FingerprintConfig) *FingerprintConfigInfo {
	return &FingerprintConfigInfo{
		config: config,
	}
}
