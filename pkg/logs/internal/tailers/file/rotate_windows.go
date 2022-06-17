// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package file

import (
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
		return false, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		log.Debugf("Error calling stat() on file %v", err)
		return false, err
	}

	// It is important to gather these values in this order, as both the file
	// size and read offset may be changing concurrently.  However, the offset
	// increases monotonically, and increments occur _after_ the file size has
	// increased, so the check that size < offset is valid as long as size is
	// polled before the offset.
	sz := st.Size()
	offset := t.lastReadOffset.Load()

	if sz < offset {
		return true, nil
	}

	return false, nil
}
