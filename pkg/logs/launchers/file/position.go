// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"io"
	"strconv"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// fileSize returns the size of the file at the given path. The file is opened through the provided
// FileOpener rather than stat'ed directly so that log files only reachable through the privileged
// logs client are handled the same way the file provider and the tailers handle them.
func fileSize(fileOpener opener.FileOpener, path string) (int64, error) {
	f, err := fileOpener.OpenLogFile(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

// Position returns the position from where logs should be collected.
func Position(registry auditor.Registry, identifier string, mode config.TailingMode, fingerprinter tailer.Fingerprinter, fileOpener opener.FileOpener) (int64, int, error) {
	var offset int64
	var whence int
	var err error

	value := registry.GetOffset(identifier)

	filePath := ""
	if len(identifier) > 5 {
		filePath = identifier[5:]
	}

	fingerprintsAlign := true

	if filePath != "" {
		prevFingerprint := registry.GetFingerprint(identifier)
		if prevFingerprint != nil {
			newFingerprint, err := fingerprinter.ComputeFingerprintFromConfig(filePath, prevFingerprint.Config)
			if err != nil {
				log.Warnf("Failed to compute fingerprint for file %s: %v", filePath, err)
				// If fingerprint computation fails, assume fingerprints don't align to be safe
				fingerprintsAlign = true
			} else {
				fingerprintsAlign = prevFingerprint.Equals(newFingerprint)
			}
		}
	}

	// A stored offset that lies beyond the end of the file means the file was rotated or truncated
	// while no tailer was watching it -- typically across an Agent restart. The equivalent check for
	// a running tailer lives in DidRotate(), but a brand-new tailer has no such protection: it would
	// seek past EOF and read nothing. When fingerprinting is enabled the launcher consults
	// DidRotateViaFingerprint() instead of DidRotate(), and that reports "no rotation" for a file
	// whose head is unchanged, so nothing would ever repair the position and the source would stay
	// at "Bytes Read: 0" indefinitely.
	offsetBeyondEOF := false
	if filePath != "" && value != "" {
		if storedOffset, perr := strconv.ParseInt(value, 10, 64); perr == nil {
			size, serr := fileSize(fileOpener, filePath)
			switch {
			case serr != nil:
				// The size could not be determined, so the offset cannot be validated. Leave the
				// stored offset in place rather than re-reading the file from the start.
				log.Warnf("Could not determine the size of file %s to validate its stored offset: %v", filePath, serr)
			case storedOffset > size:
				log.Infof("Stored offset %d for file %s is beyond its size %d, restarting from the beginning of the file", storedOffset, filePath, size)
				offsetBeyondEOF = true
			}
		}
	}

	switch {
	case mode == config.ForceBeginning:
		offset, whence = 0, io.SeekStart
	case mode == config.ForceEnd:
		offset, whence = 0, io.SeekEnd
	case value != "" && fingerprintsAlign && !offsetBeyondEOF:
		// an offset was registered, tailing mode is not forced, fingerprints are disabled or equivalent
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
	case value != "" && (!fingerprintsAlign || offsetBeyondEOF):
		// Rotation detected -- either the fingerprints don't align, or the stored offset is beyond
		// the end of the file. Start from the beginning regardless of mode.
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
