// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package file

import (
	"os"
)

// DidRotate returns true if the file has been log-rotated.
// When a log rotation occurs, the file can be either:
// - renamed and recreated
// - removed and recreated
// - truncated
func DidRotate(file *os.File, lastReadOffset int64) (bool, error) {
	f, err := openFile(file.Name())
	defer f.Close()
	if err != nil {
		return false, err
	}

	fi1, err := f.Stat()
	if err != nil {
		return false, err
	}

	fi2, err := file.Stat()
	if err != nil {
		return true, nil
	}

	recreated := !os.SameFile(fi1, fi2)
	truncated := fi1.Size() < lastReadOffset

	return recreated || truncated, nil
}
