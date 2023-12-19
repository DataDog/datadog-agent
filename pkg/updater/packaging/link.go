// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package packaging

import (
	"os"
	"path/filepath"

	"github.com/google/renameio/v2"
)

func linkRead(linkPath string) (string, error) {
	return filepath.EvalSymlinks(linkPath)
}

func linkExists(linkPath string) (bool, error) {
	_, err := os.Stat(linkPath)
	if err != nil && os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

func linkSet(linkPath string, targetPath string) error {
	return renameio.Symlink(targetPath, linkPath)
}

func linkDelete(linkPath string) error {
	return os.Remove(linkPath)
}
