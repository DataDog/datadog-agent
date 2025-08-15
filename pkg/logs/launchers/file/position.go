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

	if fingerprinter.IsFingerprintingEnabled() && filePath != "" {
		prevFingerprint := registry.GetFingerprint(identifier)
		if prevFingerprint != nil {
			newFingerprint := fingerprinter.ComputeFingerprintFromConfig(filePath, prevFingerprint.Config)
			fingerprintsAlign = prevFingerprint.Value == newFingerprint.Value
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
	case mode == config.Beginning:
		offset, whence = 0, io.SeekStart
	case mode == config.End:
		fallthrough
	default:
		offset, whence = 0, io.SeekEnd
	}
	return offset, whence, err
}
