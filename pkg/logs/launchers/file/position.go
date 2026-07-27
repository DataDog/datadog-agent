// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"io"
	"os"
	"strconv"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Position returns the position from where logs should be collected.
func Position(registry auditor.Registry, identifier string, mode config.TailingMode, fingerprinter tailer.Fingerprinter) (int64, int, error) {
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
			if fi, serr := os.Stat(filePath); serr == nil && storedOffset > fi.Size() {
				log.Infof("Stored offset %d for file %s is beyond its size %d, restarting from the beginning of the file", storedOffset, filePath, fi.Size())
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
