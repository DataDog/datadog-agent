// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package file

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DidRotate returns true if the file has been log-rotated.
//
// On *nix, when a log rotation occurs, the file can be either:
// - renamed and recreated
// - removed and recreated
// - truncated
func (t *Tailer) DidRotate() (bool, error) {
	f, err := filesystem.OpenShared(t.fullpath)
	if err != nil {
		return false, fmt.Errorf("open %q: %w", t.fullpath, err)
	}
	defer f.Close()
	lastReadOffset := t.lastReadOffset.Load()

	fi1, err := f.Stat()
	if err != nil {
		return false, fmt.Errorf("stat %q: %w", f.Name(), err)
	}

	fi2, err := t.osFile.Stat()
	if err != nil {
		return true, nil
	}

	fileSize := fi1.Size()

	recreated := !os.SameFile(fi1, fi2)    //if fingerprint changed, treat as rotation
	truncated := fileSize < lastReadOffset //if offset decreases, treat as truncation and reset reading position

	if recreated {
		log.Debugf("File rotation detected due to recreation, f1: %+v, f2: %+v", fi1, fi2)
	} else if truncated {
		log.Debugf("File rotation detected due to size change, lastReadOffset=%d, fileSize=%d", lastReadOffset, fileSize)
	}

	return recreated || truncated, nil
}

// DidRotateViaFingerprint returns true if the file has been log-rotated via fingerprint.
//
// On *nix, when a log rotation occurs, the file can be either:
// - renamed and recreated
// - removed and recreated
// - truncated
func (t *Tailer) DidRotateViaFingerprint() (bool, error) {
	newFingerprint := t.ComputeFingerPrint(t.fingerprintConfig)

	// If the old fingerprint is 0, we can't know for sure.
	// This can happen if the file was empty when the tailer started.
	if t.fingerprint == 0 {
		return false, nil
	}
	// If fingerprints are different, it means the file was rotated.
	// This is also true if the new fingerprint is 0, which means the file was truncated.
	return newFingerprint != t.fingerprint, nil
}
