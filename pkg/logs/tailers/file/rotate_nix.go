// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package file

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DidRotateWithCause reports whether the file has been log-rotated, and what
// kind of rotation it was.
//
// On *nix, when a log rotation occurs, the file can be either:
// - renamed and recreated
// - removed and recreated
// - truncated
func (t *Tailer) DidRotateWithCause() (RotationCause, error) {
	f, err := t.fileOpener.OpenLogFile(t.fullpath)
	if err != nil {
		return NoRotation, fmt.Errorf("open %q: %w", t.fullpath, err)
	}
	defer f.Close()
	lastReadOffset := t.lastReadOffset.Load()

	fi1, err := f.Stat()
	if err != nil {
		return NoRotation, fmt.Errorf("stat %q: %w", f.Name(), err)
	}

	fi2, err := t.osFile.Stat()
	if err != nil {
		// The file the tailer had open is gone, so it was replaced rather than
		// rewritten in place.
		return RotationRecreated, nil
	}

	fileSize := fi1.Size()
	t.detectAndRecordRotationSizeMismatches(fileSize, lastReadOffset)

	recreated := !os.SameFile(fi1, fi2)
	truncated := fileSize < lastReadOffset

	if recreated {
		log.Debugf("File rotation detected due to recreation, f1: %+v, f2: %+v", fi1, fi2)
		metrics.TlmRotationsNix.Inc("new_file")
		return RotationRecreated, nil
	}
	if truncated {
		log.Debugf("File rotation detected due to size change, lastReadOffset=%d, fileSize=%d", lastReadOffset, fileSize)
		metrics.TlmRotationsNix.Inc("truncated")
		return RotationTruncated, nil
	}

	return NoRotation, nil
}
