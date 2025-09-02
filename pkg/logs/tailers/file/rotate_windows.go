// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package file

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DidRotate returns true if the file has been log-rotated.
//
// On Windows, log rotation is identified by the file size being smaller
// than the last offset read.
func (t *Tailer) DidRotate() (bool, error) {
	f, err := filesystem.OpenShared(t.fullpath)
	if err != nil {
		return false, fmt.Errorf("open %q: %w", t.fullpath, err)
	}
	defer f.Close()
	offset := t.lastReadOffset.Load()

	st, err := f.Stat()
	if err != nil {
		return false, fmt.Errorf("stat %q: %w", f.Name(), err)
	}

	// It is important to gather these values in this order, as both the file
	// size and read offset may be changing concurrently.  However, the offset
	// increases monotonically, and increments occur _after_ the file size has
	// increased, so the check that size < offset is valid as long as size is
	// polled before the offset.
	sz := st.Size()

	if sz < offset {
		log.Debugf("File rotation detected due to size change, lastReadOffset=%d, fileSize=%d", offset, sz)
		return true, nil
	}

	return false, nil
}

// DidRotateViaFingerprint returns true if the file has been log-rotated via fingerprint.
//
// On windows, when a log rotation occurs, the file can be either:
// - renamed and recreated
// - removed and recreated
// - truncated
func (t *Tailer) DidRotateViaFingerprint(fingerprinter *Fingerprinter) (bool, error) {
	newFingerprint, err := fingerprinter.ComputeFingerprint(t.file)

	// If computing the fingerprint led to an error there was likely an IO issue, handle this appropriately below.
	if err != nil {
		return false, err
	}
	// If the original fingerprint is nil, we can't detect rotation
	if t.fingerprint == nil {
		return false, nil
	}

	// If fingerprints are different, it means the file was rotated.
	// This is also true if the new fingerprint is invalid (Value=0), which means the file was truncated.
	return !t.fingerprint.Equals(newFingerprint), nil
}
