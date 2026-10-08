// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package file

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// EnsureConfigFromExample initializes a missing config from its .example file.
// Existing files, including symlinks, are left untouched. Missing examples are allowed.
func EnsureConfigFromExample(rootPath, configPath string) error {
	root, err := os.OpenRoot(rootPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()

	if _, err := root.Lstat(configPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	examplePath := configPath + ".example"
	info, err := root.Lstat(examplePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("config example is not a regular file: %s", examplePath)
	}

	example, err := root.OpenFile(examplePath, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer example.Close()
	content, err := io.ReadAll(example)
	if err != nil {
		return err
	}

	// Create only if absent, never overwrite an existing config.
	config, err := root.OpenFile(configPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0400)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := config.Write(content); err != nil {
		config.Close()
		return err
	}
	return config.Close()
}
