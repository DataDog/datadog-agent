// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"errors"
	"io"
	"strconv"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// offsetBeyondEndOfFile reports whether the given offset lies past the end of the file at path.
//
// It probes the byte immediately before the offset instead of comparing the offset against the size
// reported by stat. File metadata can be cached or lag behind writes, notably on network
// filesystems, and a stale size would make a perfectly valid offset look out of range -- causing the
// file to be re-read from the start and its contents to be sent twice. A successful read proves the
// data is really there. The file is opened through the FileOpener so that log files only reachable
// through the privileged logs client are handled the same way the file provider and the tailers
// handle them.
func offsetBeyondEndOfFile(fileOpener opener.FileOpener, path string, offset int64) (bool, error) {
	if offset <= 0 {
		return false, nil
	}

	f, err := fileOpener.OpenLogFile(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.ReadAt(make([]byte, 1), offset-1)
	switch {
	case err == nil:
		// The last byte the offset accounts for is readable, so the offset is within the file.
		return false, nil
	case errors.Is(err, io.EOF):
		// The file has fewer bytes than the offset claims were already read from it.
		return true, nil
	default:
		return false, err
	}
}

// recoveryFingerprint fingerprints the file as it is now under the stored checksum
// parameters, so the result is comparable with the stored fingerprint.
func recoveryFingerprint(fingerprinter tailer.Fingerprinter, filePath string, storedConfig *types.FingerprintConfig, currentFingerprint *types.Fingerprint) (*types.Fingerprint, error) {
	if storedConfig == nil || currentFingerprint == nil || currentFingerprint.Config == nil {
		return fingerprinter.ComputeFingerprintFromConfig(filePath, storedConfig)
	}

	// Reuse avoids a second read that could catch the file mid-rotation. An invalid
	// fingerprint means no read happened, so reusing it would falsely signal a rotation.
	if currentFingerprint.ValidFingerprint() && storedConfig.SameChecksumParameters(currentFingerprint.Config) {
		return currentFingerprint, nil
	}

	// Re-read under the stored parameters, but with the currently configured open mode.
	recoveryConfig := *storedConfig
	recoveryConfig.OpenFlags = append([]types.FileOpenFlag(nil), currentFingerprint.Config.OpenFlags...)
	return fingerprinter.ComputeFingerprintFromConfig(filePath, &recoveryConfig)
}

// Position returns the position from where logs should be collected.
func Position(registry auditor.Registry, identifier string, mode config.TailingMode, fingerprinter tailer.Fingerprinter, fileOpener opener.FileOpener, currentFingerprint *types.Fingerprint) (int64, int, error) {
	var offset int64
	var whence int
	var err error

	value := registry.GetOffset(identifier)

	filePath := ""
	if len(identifier) > 5 {
		filePath = identifier[5:]
	}

	// isSameFile reports whether the file on disk is still the one the stored offset was recorded
	// against, and therefore whether that offset can be trusted. Two things can rule it out.
	isSameFile := true

	// The fingerprint recorded for the file no longer matches its current head content, meaning the
	// file was replaced.
	if filePath != "" {
		prevFingerprint := registry.GetFingerprint(identifier)
		if prevFingerprint != nil {
			newFingerprint, ferr := recoveryFingerprint(fingerprinter, filePath, prevFingerprint.Config, currentFingerprint)
			if ferr != nil {
				if currentFingerprint != nil && tailer.FingerprintOpenFlagsActive(currentFingerprint.Config) {
					return 0, 0, ferr
				}
				// The fingerprint could not be computed, so keep trusting the stored offset rather
				// than re-reading the file from the start and sending its contents twice.
				log.Warnf("Failed to compute fingerprint for file %s: %v", filePath, ferr)
			} else {
				isSameFile = prevFingerprint.Equals(newFingerprint)
			}
		}
	}

	// Or the stored offset lies beyond the end of the file, meaning it was rotated or truncated while
	// no tailer was watching it, typically across an Agent restart. A running tailer is protected from
	// that by DidRotate(), but a tailer that is only just starting is not: it would seek past the end
	// of the file and read nothing. Nothing repairs it afterwards when fingerprinting is enabled,
	// because the launcher then consults DidRotateViaFingerprint(), which reports "no rotation" for a
	// file whose head content is unchanged -- so the source would stay at "Bytes Read: 0" indefinitely.
	if isSameFile && filePath != "" && value != "" {
		if storedOffset, perr := strconv.ParseInt(value, 10, 64); perr == nil {
			beyondEOF, cerr := offsetBeyondEndOfFile(fileOpener, filePath, storedOffset)
			switch {
			case cerr != nil:
				// Same reasoning as above: an offset that cannot be checked is left alone.
				log.Warnf("Could not check whether the stored offset for file %s is still within it: %v", filePath, cerr)
			case beyondEOF:
				log.Infof("Stored offset %d for file %s is beyond the end of the file, restarting from the beginning of the file", storedOffset, filePath)
				isSameFile = false
			}
		}
	}

	switch {
	case mode == config.ForceBeginning:
		offset, whence = 0, io.SeekStart
	case mode == config.ForceEnd:
		offset, whence = 0, io.SeekEnd
	case value != "" && isSameFile:
		// an offset was registered, tailing mode is not forced, and the file it was recorded against
		// is still the one on disk
		whence = io.SeekStart
		offset, err = strconv.ParseInt(value, 10, 64)
		if err != nil {
			offset = 0
			if mode == config.End {
				whence = io.SeekEnd
			} else if mode == config.Beginning {
				whence = io.SeekStart
			}
		}
	case value != "" && !isSameFile:
		// Rotation detected, start from the beginning regardless of mode
		offset, whence = 0, io.SeekStart
	case mode == config.Beginning:
		offset, whence = 0, io.SeekStart
	case mode == config.End:
		fallthrough
	default:
		offset, whence = 0, io.SeekEnd
	}
	return offset, whence, err
}
