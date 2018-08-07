// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package file

import (
	"io"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/seek"
)

// Position returns the position from where logs should be collected.
func Position(seeker *seek.Seeker, ctime time.Time, identifier string) (int64, int, error) {
	var offset int64
	var whence int
	var err error
	strategy, value := seeker.Seek(ctime, identifier)
	switch strategy {
	case seek.Start:
		offset, whence = 0, io.SeekStart
	case seek.Recover:
		offset, err = strconv.ParseInt(value, 10, 64)
		if err != nil {
			offset, whence = 0, io.SeekEnd
		} else {
			whence = io.SeekStart
		}
	case seek.End:
		offset, whence = 0, io.SeekEnd
	}
	return offset, whence, err
}
