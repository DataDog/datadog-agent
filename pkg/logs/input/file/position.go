// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package file

import (
	"io"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
)

// Position returns the position from where logs should be collected.
func Position(registry auditor.Registry, identifier string, tailFromBeginning bool) (int64, int, error) {
	var offset int64
	var whence int
	var err error
	value := registry.GetOffset(identifier)
	switch {
	case value != "":
		// an offset was registered, tail from the offset
		whence = io.SeekStart
		offset, err = strconv.ParseInt(value, 10, 64)
		if err != nil {
			offset, whence = 0, io.SeekEnd
		}
	case tailFromBeginning:
		// a new service has been discovered, tail from the beginning
		offset, whence = 0, io.SeekStart
	default:
		// a new config has been discovered, tail from the end
		offset, whence = 0, io.SeekEnd
	}
	return offset, whence, err
}
