// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package file

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
)

// EnsureConfigFromExample initializes a missing config from its .example file.
// Existing files, including symlinks, are left untouched. Missing examples are allowed.
func EnsureConfigFromExample(rootPath, configPath string) (err error) {
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

	// Nonblocking open avoids hanging on a FIFO substituted for the example.
	example, err := root.OpenFile(examplePath, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer example.Close()
	openedInfo, err := example.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(info, openedInfo) {
		return fmt.Errorf("config example changed while opening: %s", examplePath)
	}

	tmpPath := configPath + ".tmp-" + rand.Text()
	tmp, err := root.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0400)
	if err != nil {
		return err
	}
	defer func() {
		if removeErr := root.Remove(tmpPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("failed to remove temporary config: %w", removeErr))
		}
	}()
	defer tmp.Close()
	if _, err := io.Copy(tmp, example); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// Publish complete content atomically, without replacing a concurrent writer.
	if err := root.Link(tmpPath, configPath); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return nil
}
