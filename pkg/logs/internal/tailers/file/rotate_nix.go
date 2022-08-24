// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package file

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

// DidRotate returns true if the file has been log-rotated.
//
// On *nix, when a log rotation occurs, the file can be either:
// - renamed and recreated
// - removed and recreated
// - truncated
func (t *Tailer) DidRotate() (bool, error) {
	f, err := filesystem.OpenShared(t.osFile.Name())
	if err != nil {
		return false, err
	}
	defer f.Close()

	fi1, err := f.Stat()
	if err != nil {
		return false, err
	}

	fi2, err := t.osFile.Stat()
	if err != nil {
		return true, nil
	}

	recreated := !os.SameFile(fi1, fi2)
	truncated := fi1.Size() < t.lastReadOffset.Load()

	return recreated || truncated, nil
}
