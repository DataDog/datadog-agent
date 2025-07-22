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
)

// Position returns the position from where logs should be collected.
func Position(registry auditor.Registry, identifier string, mode config.TailingMode, fingerprintStrategy string) (int64, int, error) {
	var offset int64
	var whence int
	var err error

	value := registry.GetOffset(identifier)
	fingerprintConfig := registry.GetFingerprintConfig(identifier)
	previousFingerprint := registry.GetFingerprint(identifier)
	filePath := ""
	if len(identifier) > 5 {
		filePath = identifier[5:]
	}
	newFingerprint := tailerfile.ComputeFingerprint(filePath, fingerprintConfig)

	// Check if fingerprinting is currently enabled based on the provided strategy
	fingerprintingEnabled := fingerprintStrategy == "checksum"

	switch {
	case mode == config.ForceBeginning:
		offset, whence = 0, io.SeekStart
	case mode == config.ForceEnd:
		offset, whence = 0, io.SeekEnd
	case value != "" && fingerprintingEnabled && (previousFingerprint == newFingerprint || previousFingerprint == 0): // fingerprint valid and fingerprinting is currently enabled
		// an offset was registered, tailing mode is not forced, fingerprinting is enabled, and fingerprint is valid
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
	case value != "" && !fingerprintingEnabled:
		// an offset was registered but fingerprinting is disabled - use offset without fingerprint validation
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
	case mode == config.Beginning:
		offset, whence = 0, io.SeekStart
	case mode == config.End:
		fallthrough
	default:
		offset, whence = 0, io.SeekEnd
	}
	return offset, whence, err
}
