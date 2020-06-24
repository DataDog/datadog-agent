// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package file

import (
	"io"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

// Position returns the position from where logs should be collected.
func Position(registry auditor.Registry, identifier string, mode config.TailingMode) (int64, int, error) {
	var offset int64
	var whence int
	var err error

	value := registry.GetOffset(identifier)

	switch {
	case mode == config.ForceBeginning:
		offset, whence = 0, io.SeekStart
	case mode == config.ForceEnd:
		offset, whence = 0, io.SeekEnd
	case value != "":
		// an offset was registered, tailing mode is not forced, tail from the offset
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
