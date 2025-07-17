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
	tailerfile "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Position returns the position from where logs should be collected.
func Position(registry auditor.Registry, identifier string, mode config.TailingMode) (int64, int, error) {
	var offset int64
	var whence int
	var err error

	log.Debugf("Position calculation started for identifier: %s, mode: %v", identifier, mode)

	value := registry.GetOffset(identifier)
	fingerprintConfig := registry.GetFingerprintConfig(identifier)
	previousFingerprint := registry.GetFingerprint(identifier)
	filePath := ""
	if len(identifier) > 5 {
		filePath = identifier[5:]
	}
	newFingerprint := tailerfile.ComputeFingerprint(filePath, fingerprintConfig)

	log.Debugf("Position calculation for identifier %s (filePath: %s, mode: %v)", identifier, filePath, mode)
	log.Debugf("  - Stored offset: %s", value)
	log.Debugf("  - Fingerprint config: %v", fingerprintConfig != nil)
	log.Debugf("  - Previous fingerprint: 0x%x", previousFingerprint)
	log.Debugf("  - New fingerprint: 0x%x", newFingerprint)

	switch {
	case mode == config.ForceBeginning:
		log.Debugf("Using ForceBeginning mode - offset: 0, whence: SeekStart")
		offset, whence = 0, io.SeekStart
	case mode == config.ForceEnd:
		log.Debugf("Using ForceEnd mode - offset: 0, whence: SeekEnd")
		offset, whence = 0, io.SeekEnd
	case value != "" && previousFingerprint == newFingerprint: //and fingerprint valid (fingerprint stored oldconfig and recalculate oldconfig the same)
		log.Debugf("Using stored offset with valid fingerprint - offset: %s, whence: SeekStart", value)
		// an offset was registered, tailing mode is not forced, tail from the offset
		whence = io.SeekStart
		offset, err = strconv.ParseInt(value, 10, 64)
		if err != nil {
			log.Debugf("Failed to parse stored offset '%s': %v, falling back to mode-based positioning", value, err)
			offset = 0
			if mode == config.End {
				whence = io.SeekEnd
				log.Debugf("Falling back to End mode due to offset parse error")
			} else if mode == config.Beginning {
				whence = io.SeekStart
				log.Debugf("Falling back to Beginning mode due to offset parse error")
			}
		} else {
			log.Debugf("Successfully parsed stored offset: %d", offset)
		}
	case mode == config.Beginning:
		log.Debugf("Using Beginning mode - offset: 0, whence: SeekStart")
		offset, whence = 0, io.SeekStart
	case mode == config.End:
		log.Debugf("Using End mode (default) - offset: 0, whence: SeekEnd")
		fallthrough
	default:
		offset, whence = 0, io.SeekEnd
	}

	log.Debugf("Final position for %s: offset=%d, whence=%d", identifier, offset, whence)
	return offset, whence, err
}
