// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

var rollbackNoop = func() error { return nil }

// fileMutator is a struct used to transform a file
// creating backups, replacing original files and setting permissions
// default permissions are root:root 0644
type fileMutator struct {
	path             string
	pathTmp          string
	pathBackup       string
	transformContent func(ctx context.Context, existing []byte) ([]byte, error)
	validateTemp     func() error
	validateFinal    func() error
}

// newFileMutator creates a new fileMutator
func newFileMutator(path string, transform func(ctx context.Context, existing []byte) ([]byte, error), validateTemp, validateFinal func() error) *fileMutator {
	return &fileMutator{
		path:             path,
		pathTmp:          path + ".datadog.prep",
		pathBackup:       path + ".datadog.backup",
		transformContent: transform,
		validateTemp:     validateTemp,
		validateFinal:    validateFinal,
	}
}

func (ft *fileMutator) mutate(ctx context.Context) (rollback func() error, err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "mutate_file")
	defer func() { span.Finish(tracer.WithError(err)) }()
	span.SetTag("file", ft.path)

	defer os.Remove(ft.pathTmp)

	originalFileExists := true
	// create backup and temporary file if the original file exists
	if _, err := os.Stat(ft.path); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("could not stat file %s: %s", ft.path, err)
		}
		originalFileExists = false
	}
	if originalFileExists {
		if err := copyFile(ft.path, ft.pathBackup); err != nil {
			return nil, fmt.Errorf("could not create backup file %s: %s", ft.pathBackup, err)
		}
		if err := copyFile(ft.pathBackup, ft.pathTmp); err != nil {
			return nil, fmt.Errorf("could not create temporary file %s: %s", ft.pathTmp, err)
		}
	}

	data, err := os.ReadFile(ft.pathTmp)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("could not read file %s: %s", ft.pathTmp, err)
	}

	res, err := ft.transformContent(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("could not transform file %s: %s", ft.pathTmp, err)
	}

	// no changes needed
	if bytes.Equal(data, res) {
		return rollbackNoop, nil
	}

	if err := writeFile(ft.pathTmp, res); err != nil {
		return nil, fmt.Errorf("could not write file %s: %s", ft.pathTmp, err)

	}

	// validate temporary file if validation function provided
	if ft.validateTemp != nil {
		if err := ft.validateTemp(); err != nil {
			return nil, fmt.Errorf("could not validate temporary file %s: %s", ft.pathTmp, err)
		}
	}

	if err := os.Rename(ft.pathTmp, ft.path); err != nil {
		return nil, fmt.Errorf("could not rename temporary file %s to %s: %s", ft.pathTmp, ft.path, err)
	}

	// prepare rollback function
	rollback = func() error {
		if originalFileExists {
			return os.Rename(ft.pathBackup, ft.path)
		}
		return os.Remove(ft.path)
	}

	// validate final file if validation function provided
	if ft.validateFinal != nil {
		if err = ft.validateFinal(); err != nil {
			if err := rollback(); err != nil {
				log.Errorf("could not rollback file %s: %s", ft.path, err)
			}
			return nil, err
		}
	}
	return rollback, nil
}

func writeFile(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	if err != nil {
		return err
	}
	// flush in-memory file system to disk
	if err = f.Sync(); err != nil {
		return err
	}
	return nil
}

func copyFile(src, dst string) (err error) {
	defer func() {
		if err != nil {
			os.Remove(dst)
		}
	}()

	var srcFile, dstFile *os.File
	srcFile, err = os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// get permissions and ownership
	var srcInfo os.FileInfo
	srcInfo, err = srcFile.Stat()
	if err != nil {
		return err
	}
	var stat *syscall.Stat_t
	var ok bool
	stat, ok = srcInfo.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return fmt.Errorf("could not get file stat")
	}

	// create dst file with same permissions
	dstFile, err = os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// copy content
	if _, err = io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	// set ownership
	if err = os.Chown(dst, int(stat.Uid), int(stat.Gid)); err != nil {
		return err
	}

	// flush in-memory file system to disk
	if err = dstFile.Sync(); err != nil {
		return err
	}

	return nil
}

func (ft *fileMutator) cleanup() {
	_ = os.Remove(ft.pathTmp)
	_ = os.Remove(ft.pathBackup)
}
