// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package repository

import (
	"errors"
	"os"
	"path/filepath"
)

func linkRead(linkPath string) (string, error) {
	return filepath.EvalSymlinks(linkPath)
}

func linkExists(linkPath string) (bool, error) {
	_, err := os.Stat(linkPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

func linkSet(linkPath string, targetPath string) error {
	return atomicSymlink(targetPath, linkPath)
}

func linkDelete(linkPath string) error {
	return os.Remove(linkPath)
}
