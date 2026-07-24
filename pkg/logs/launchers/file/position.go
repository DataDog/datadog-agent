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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Position returns the position from where logs should be collected.
func Position(registry auditor.Registry, identifier string, mode config.TailingMode, fingerprinter tailer.Fingerprinter, file *tailer.File) (int64, int, error) {
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
			switch {
			case prevFingerprint.ValidFingerprint():
				// The stored fingerprint carries a real value, so recompute using the *stored*
				// config to keep the comparison apples-to-apples, then trust the saved offset only
				// if the recomputed fingerprint still matches.
				newFingerprint, ferr := fingerprinter.ComputeFingerprintFromConfig(filePath, prevFingerprint.Config)
				if ferr != nil {
					log.Warnf("Failed to compute fingerprint for file %s: %v", filePath, ferr)
					// The file identity could not be verified, so do not trust the stored offset.
					fingerprintsAlign = false
				} else {
					fingerprintsAlign = prevFingerprint.Equals(newFingerprint)
				}
			case file != nil && fingerprinter.ShouldFileFingerprint(file):
				// The stored fingerprint is invalid (Value 0) because it predates the source's
				// fingerprint_config or was written with a disabled strategy, yet the source now has
				// active fingerprinting. Its Value cannot be meaningfully compared against a freshly
				// computed one (0 trivially equals 0), so the offset cannot be verified. Treat it as
				// misaligned so the tailer restarts from the beginning of the (possibly rotated)
				// file, letting the source self-heal instead of reusing a stale, out-of-range offset.
				fingerprintsAlign = false
			}
		}
	}

	switch {
	case mode == config.ForceBeginning:
		offset, whence = 0, io.SeekStart
	case mode == config.ForceEnd:
		offset, whence = 0, io.SeekEnd
	case value != "" && fingerprintsAlign:
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
	case !fingerprintsAlign && value != "":
		// Fingerprints don't align (rotation detected), start from beginning regardless of mode
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
