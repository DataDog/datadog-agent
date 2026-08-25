// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"bufio"
	"errors"
	"fmt"
	"hash/crc64"
	"io"
	"runtime"
	"strings"
	"sync"

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
	// GetEffectiveConfigForFile returns the fingerprint configuration that applies to a file for status display purposes
	GetEffectiveConfigForFile(file *File) *types.FingerprintConfig
	// ForgetOpenFlagsUnsupported drops memoized open-flag fallback state for the
	// given memo keys. A file is memoized under both its tailer scan key and its
	// path (see the openFlagsUnsupported field), so the file launcher passes both.
	ForgetOpenFlagsUnsupported(memoKeys ...string)
}

// fingerprinterImpl is a struct that contains the fingerprinting configuration
type fingerprinterImpl struct {
	globalConfig types.FingerprintConfig
	fileOpener   opener.FileOpener
	// openFlagsUnsupported records memo keys whose configured open_flags could not
	// be used, whether because the filesystem rejected them or because the Agent
	// cannot open that file directly. Recorded keys skip straight to a buffered
	// open instead of retrying a failing one, and being recorded doubles as the
	// dedupe for the one-time fallback warning.
	//
	// ComputeFingerprint keys by scan key so distinct tailers on the same path
	// (e.g. container rotation) do not share state. ComputeFingerprintFromConfig
	// keys by path because the registry identifier has no container suffix. Those
	// two keys differ for container sources, so the file launcher forgets both
	// when it stops a tailer. The bounded cache also covers candidate files that
	// disappear or fail before the launcher can create and later stop a tailer.
	openFlagsUnsupported openFlagsUnsupportedCache
}

const maxOpenFlagsUnsupportedEntries = 1024

// openFlagsUnsupportedCache bounds fallback memoization so short-lived files
// that never produce a tailer cannot grow the Agent's memory for its lifetime.
// Entries are kept in insertion order; lifecycle cleanup removes active files,
// and the oldest orphan is evicted if the cache reaches its limit.
type openFlagsUnsupportedCache struct {
	mu         sync.Mutex
	maxEntries int
	entries    map[string]struct{}
	order      []string
}

func newOpenFlagsUnsupportedCache(maxEntries int) openFlagsUnsupportedCache {
	if maxEntries <= 0 {
		maxEntries = maxOpenFlagsUnsupportedEntries
	}
	return openFlagsUnsupportedCache{
		maxEntries: maxEntries,
		entries:    make(map[string]struct{}, maxEntries),
		order:      make([]string, 0, maxEntries),
	}
}

func (c *openFlagsUnsupportedCache) initializeLocked() {
	if c.maxEntries <= 0 {
		c.maxEntries = maxOpenFlagsUnsupportedEntries
	}
	if c.entries == nil {
		c.entries = make(map[string]struct{}, c.maxEntries)
	}
}

func (c *openFlagsUnsupportedCache) contains(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, found := c.entries[key]
	return found
}

// add records key and reports whether it was newly added.
func (c *openFlagsUnsupportedCache) add(key string) bool {
	if key == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initializeLocked()
	if _, found := c.entries[key]; found {
		return false
	}
	if len(c.entries) >= c.maxEntries {
		oldest := c.order[0]
		c.order[0] = ""
		c.order = c.order[1:]
		delete(c.entries, oldest)
	}
	c.entries[key] = struct{}{}
	c.order = append(c.order, key)
	return true
}

func (c *openFlagsUnsupportedCache) delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, found := c.entries[key]; !found {
		return
	}
	delete(c.entries, key)
	for i, cachedKey := range c.order {
		if cachedKey != key {
			continue
		}
		copy(c.order[i:], c.order[i+1:])
		c.order[len(c.order)-1] = ""
		c.order = c.order[:len(c.order)-1]
		return
	}
}

func (c *openFlagsUnsupportedCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// FingerprintConfigInfo holds fingerprint configuration for status display
type FingerprintConfigInfo struct {
	config *types.FingerprintConfig
}

// NewFingerprinter creates a new Fingerprinter with the given configuration
func NewFingerprinter(fingerprintConfig types.FingerprintConfig, opener opener.FileOpener) Fingerprinter {
	return &fingerprinterImpl{
		globalConfig:         fingerprintConfig,
		fileOpener:           opener,
		openFlagsUnsupported: newOpenFlagsUnsupportedCache(maxOpenFlagsUnsupportedEntries),
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
func (f *fingerprinterImpl) ShouldFileFingerprint(file *File) bool {
	fileFingerprintConfig := file.Source.Config().FingerprintConfig

	// Check per-source config first (takes precedence over global config)
	if fileFingerprintConfig != nil {
		if fileFingerprintConfig.FingerprintStrategy == types.FingerprintStrategyDisabled {
			return false
		}
		if fileFingerprintConfig.FingerprintStrategy != "" {
			return true
		}
	}

	// Now, check global config
	return f.globalConfig.FingerprintStrategy != types.FingerprintStrategyDisabled
}

// ComputeFingerprintFromConfig computes the fingerprint for the given file path using a specific config
// Note that the provided configuration can fallback to different default configuration if specific errors occur attempting to compute the fingerprint.
func (f *fingerprinterImpl) ComputeFingerprintFromConfig(filepath string, fingerprintConfig *types.FingerprintConfig) (*types.Fingerprint, error) {
	if fingerprintConfig != nil && fingerprintConfig.FingerprintStrategy == types.FingerprintStrategyDisabled {
		return newInvalidFingerprint(fingerprintConfig), nil
	}
	return f.computeFingerprintAtPath(filepath, fingerprintConfig)
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

	fileFingerprintConfig := file.Source.Config().FingerprintConfig

	// Check per-source config first (takes precedence over global config)
	if fileFingerprintConfig != nil && fileFingerprintConfig.FingerprintStrategy != "" {
		// Convert from config.FingerprintConfig to types.FingerprintConfig
		// This must happen before checking if fingerprinting is disabled so the Source field is always set
		fingerprintConfig := &types.FingerprintConfig{
			FingerprintStrategy: types.FingerprintStrategy(fileFingerprintConfig.FingerprintStrategy),
			Count:               fileFingerprintConfig.Count,
			CountToSkip:         fileFingerprintConfig.CountToSkip,
			MaxBytes:            fileFingerprintConfig.MaxBytes,
			OpenFlags:           append([]types.FileOpenFlag(nil), fileFingerprintConfig.OpenFlags...),
			Source:              types.FingerprintConfigSourcePerSource,
		}

		if fileFingerprintConfig.FingerprintStrategy == types.FingerprintStrategyDisabled {
			return newInvalidFingerprint(fingerprintConfig), nil
		}

		return f.computeFingerprintForFile(file, fingerprintConfig)
	}

	// If per-source config exists but no strategy is set, or no per-source config exists,
	// fall back to global config
	return f.computeFingerprintForFile(file, &f.globalConfig)
}

// ComputeFingerprintFromHandle computes the fingerprint for the given os.File using the provided config.
// Note that the providedconfiguration can fallback to different default configuration if specific errors occur attempting to compute the fingerprint.
func (f *fingerprinterImpl) ComputeFingerprintFromHandle(osFile afero.File, fingerprintConfig *types.FingerprintConfig) (*types.Fingerprint, error) {
	return f.computeFingerprintFromHandle(osFile, fingerprintConfig, nil)
}

// computeFingerprintFromHandle backs ComputeFingerprintFromHandle.
// appliedOpenFlags is the subset of configured open_flags actually used to open
// osFile (nil when the handle was opened buffered). Read errors that mean
// "retry without flags" are silenced so the caller can fall back cleanly.
func (f *fingerprinterImpl) computeFingerprintFromHandle(osFile afero.File, fingerprintConfig *types.FingerprintConfig, appliedOpenFlags []types.FileOpenFlag) (*types.Fingerprint, error) {
	if fingerprintConfig == nil {
		return newInvalidFingerprint(nil), nil
	}

	if osFile == nil {
		return newInvalidFingerprint(nil), errors.New("osFile cannot be nil")
	}

	// Get file path for logging purposes
	filePath := osFile.Name()

	// Determine fingerprinting strategy (line_checksum or byte_checksum)
	strategy := fingerprintConfig.FingerprintStrategy
	switch strategy {
	case types.FingerprintStrategyLineChecksum:
		return computeFingerPrintByLines(osFile, filePath, fingerprintConfig, appliedOpenFlags)
	case types.FingerprintStrategyByteChecksum:
		return computeFingerPrintByBytes(osFile, filePath, fingerprintConfig, appliedOpenFlags)
	default:
		log.Warnf("invalid fingerprint strategy %q for file %q, using default lines strategy", strategy, filePath)
		// Default to line_checksum if no strategy is specified
		return computeFingerPrintByLines(osFile, filePath, defaultLinesConfig, appliedOpenFlags)
	}
}

// computeFingerprintAtPath fingerprints filePath. Used when only the path is
// known (registry position recovery); memoization is keyed by path as well.
func (f *fingerprinterImpl) computeFingerprintAtPath(filePath string, fingerprintConfig *types.FingerprintConfig) (*types.Fingerprint, error) {
	return f.computeFingerprintWithMemoKey(filePath, filePath, fingerprintConfig)
}

// computeFingerprintForFile fingerprints file.Path, memoizing open-flag fallback
// under the tailer scan key so distinct tailers on the same path stay isolated.
func (f *fingerprinterImpl) computeFingerprintForFile(file *File, fingerprintConfig *types.FingerprintConfig) (*types.Fingerprint, error) {
	return f.computeFingerprintWithMemoKey(file.GetScanKey(), file.Path, fingerprintConfig)
}

// computeFingerprintWithMemoKey opens openPath and memoizes open-flag fallback
// under memoKey. The keys differ for container tailers (scan key vs path).
func (f *fingerprinterImpl) computeFingerprintWithMemoKey(memoKey, openPath string, fingerprintConfig *types.FingerprintConfig) (*types.Fingerprint, error) {
	if fingerprintConfig == nil {
		return newInvalidFingerprint(nil), nil
	}

	openFlags := fingerprintConfig.OpenFlags
	useOpenFlags := fingerprintOpenFlagsActive(openFlags)
	if useOpenFlags {
		if f.openFlagsUnsupported.contains(memoKey) {
			// The configured open_flags already proved unusable for this tailer; skip
			// the doomed flagged open and read buffered.
			useOpenFlags = false
		}
	}

	flagsToUse := openFlags
	if !useOpenFlags {
		flagsToUse = nil
	}

	fingerprint, err := f.computeFingerprintOnce(openPath, fingerprintConfig, flagsToUse)
	if !useOpenFlags || err == nil || !opener.IsOpenFlagsUnsupportedError(err) {
		return fingerprint, err
	}

	if f.openFlagsUnsupported.add(memoKey) {
		// Applies when a filesystem rejects the flags or the Agent cannot open the
		// file directly with them; keep the wording about the cause neutral.
		log.Warnf(
			"fingerprint open_flags are not usable for %q; falling back to buffered reads for this file: %v",
			openPath,
			err,
		)
	}
	return f.computeFingerprintOnce(openPath, fingerprintConfig, nil)
}

func (f *fingerprinterImpl) computeFingerprintOnce(filePath string, fingerprintConfig *types.FingerprintConfig, openFlags []types.FileOpenFlag) (*types.Fingerprint, error) {
	var fpFile afero.File
	var err error
	if fingerprintOpenFlagsActive(openFlags) {
		fpFile, err = f.fileOpener.OpenLogFileWithFlags(filePath, openFlags)
	} else {
		fpFile, err = f.fileOpener.OpenLogFile(filePath)
	}
	if err != nil {
		if shouldWarnFingerprintReadError(openFlags, err) {
			log.Warnf("could not open file for fingerprinting %s: %v", filePath, err)
		}
		return newInvalidFingerprint(fingerprintConfig), err
	}
	defer fpFile.Close()

	return f.computeFingerprintFromHandle(fpFile, fingerprintConfig, openFlags)
}

// fingerprintOpenFlagsActive reports whether configured open_flags should be
// applied for this fingerprint open. They are Linux-only; other platforms use
// OpenLogFile and ignore the configured flags.
func fingerprintOpenFlagsActive(openFlags []types.FileOpenFlag) bool {
	return len(openFlags) > 0 && runtime.GOOS == "linux"
}

// computeFingerPrintByBytes computes fingerprint using byte-based approach for a given file path
func computeFingerPrintByBytes(fpFile afero.File, filePath string, fingerprintConfig *types.FingerprintConfig, appliedOpenFlags []types.FileOpenFlag) (*types.Fingerprint, error) {
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
		if shouldWarnFingerprintReadError(appliedOpenFlags, err) {
			log.Warnf("Failed to read bytes for fingerprint %q: %v", filePath, err)
		}
		return newInvalidFingerprint(fingerprintConfig), err
	}

	// Check if we have enough bytes to create a meaningful fingerprint
	if bytesRead == 0 || bytesRead < maxBytes {
		return newInvalidFingerprint(fingerprintConfig), nil
	}

	// Compute fingerprint
	checksum := crc64.Checksum(buffer, crc64Table)

	return &types.Fingerprint{Value: checksum, Config: fingerprintConfig}, nil
}

// computeFingerPrintByLines computes fingerprint using line-based approach for a given file path
func computeFingerPrintByLines(fpFile afero.File, filePath string, fingerprintConfig *types.FingerprintConfig, appliedOpenFlags []types.FileOpenFlag) (*types.Fingerprint, error) {
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
				return computeFingerPrintByBytes(fpFile, filePath, &fallbackConfig, appliedOpenFlags)
			}
			// Handle scanner errors
			if err := scanner.Err(); err != nil {
				if shouldWarnFingerprintReadError(appliedOpenFlags, err) {
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

// shouldWarnFingerprintReadError reports whether a fingerprint read/open error
// is worth logging. When appliedOpenFlags is non-empty, an "unsupported flags"
// error is expected and handled by a buffered retry, so it is silenced.
func shouldWarnFingerprintReadError(appliedOpenFlags []types.FileOpenFlag, err error) bool {
	return len(appliedOpenFlags) == 0 || !opener.IsOpenFlagsUnsupportedError(err)
}

// ForgetOpenFlagsUnsupported drops memoized open-flag fallback state for the given
// memo keys so the next fingerprint can retry the configured flags. The file launcher
// passes both the tailer scan key and the file path when it stops a tailer; the two
// are the same string for non-container sources.
func (f *fingerprinterImpl) ForgetOpenFlagsUnsupported(memoKeys ...string) {
	for _, memoKey := range memoKeys {
		if memoKey == "" {
			continue
		}
		f.openFlagsUnsupported.delete(memoKey)
	}
}

// GetEffectiveConfigForFile returns the fingerprint configuration that applies to a file
// for status display purposes. This returns the config even when fingerprinting is disabled.
func (f *fingerprinterImpl) GetEffectiveConfigForFile(file *File) *types.FingerprintConfig {
	if file == nil {
		return nil
	}

	fileFingerprintConfig := file.Source.Config().FingerprintConfig

	// Check per-source config first (takes precedence over global config)
	if fileFingerprintConfig != nil && fileFingerprintConfig.FingerprintStrategy != "" {
		// Convert from config.FingerprintConfig to types.FingerprintConfig
		return &types.FingerprintConfig{
			FingerprintStrategy: types.FingerprintStrategy(fileFingerprintConfig.FingerprintStrategy),
			Count:               fileFingerprintConfig.Count,
			CountToSkip:         fileFingerprintConfig.CountToSkip,
			MaxBytes:            fileFingerprintConfig.MaxBytes,
			OpenFlags:           append([]types.FileOpenFlag(nil), fileFingerprintConfig.OpenFlags...),
			Source:              types.FingerprintConfigSourcePerSource,
		}
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
