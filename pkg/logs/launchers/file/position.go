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
			log.Infof("POSITION DEBUG: Found previous fingerprint for %s (value: 0x%x), computing new fingerprint", identifier, prevFingerprint.Value)
			newFingerprint, err := fingerprinter.ComputeFingerprintFromConfig(filePath, prevFingerprint.Config)
			if err != nil {
				log.Warnf("Failed to compute fingerprint for file %s: %v", filePath, err)
				// More likely to have the agent come back up pointed to the same file compared to a rotated file
				fingerprintsAlign = true
				log.Infof("POSITION DEBUG: Fingerprint computation failed, defaulting to fingerprintsAlign=true")
			} else {
				fingerprintsAlign = prevFingerprint.Value == newFingerprint.Value
				log.Infof("POSITION DEBUG: Computed new fingerprint for %s (value: 0x%x), fingerprintsAlign=%t", identifier, newFingerprint.Value, fingerprintsAlign)
			}
		} else {
			log.Infof("POSITION DEBUG: No previous fingerprint found for %s, fingerprintsAlign=true (default)", identifier)
		}
	}

	log.Debugf("Position() called for identifier: %s, mode: %d, value: %s, fingerprintsAlign: %t", identifier, mode, value, fingerprintsAlign)
	log.Debugf("config.Beginning: %d, mode == config.Beginning: %t", config.Beginning, mode == config.Beginning)
	switch {
	case mode == config.ForceBeginning:
		log.Debugf("HIT: ForceBeginning case")
		offset, whence = 0, io.SeekStart
	case mode == config.ForceEnd:
		log.Debugf("HIT: ForceEnd case")
		offset, whence = 0, io.SeekEnd
	case value != "" && fingerprintsAlign:
		log.Debugf("HIT: Saved offset case - value: %s, fingerprintsAlign: %t", value, fingerprintsAlign)
		// an offset was registered, tailing mode is not forced, fingerprints align, so we start from the offset
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
	case value != "" && !fingerprintsAlign:
		// Fingerprints don't match - likely rotation detected, start from beginning to avoid missing data
		log.Infof("ROTATION DETECTED: Fingerprints don't align for file %s (identifier: %s, saved offset: %s), starting from beginning", filePath, identifier, value)
		log.Infof("ROTATION DETECTED: Previous fingerprint exists, new fingerprint differs - this is file rotation scenario")
		offset, whence = 0, io.SeekStart
	case mode == config.Beginning:
		log.Debugf("HIT: Beginning case")
		offset, whence = 0, io.SeekStart
	case mode == config.End:
		log.Debugf("HIT: End case")
		fallthrough
	default:
		log.Debugf("HIT: Default case")
		offset, whence = 0, io.SeekEnd
	}
	return offset, whence, err
}
