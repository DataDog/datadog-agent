// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package secretsimpl is the implementation for the secrets component
package secretsimpl

import (
	"fmt"
	"os"
)

// TODO: (components) consider moving to pkg/util/filesystem

// fileSizeLimitError is the error returned when a file's size is larger than the limit
type fileSizeLimitError struct {
	Size  int64
	Limit int64
}

// Error returns a string describing the file size limit error
func (e *fileSizeLimitError) Error() string {
	return fmt.Sprintf("file larger than max size: %d > %d", e.Size, e.Limit)
}

// readFileWithSizeLimit reads a file's contents unless it is larger than the size limit
func readFileWithSizeLimit(filename string, maxSize int64) ([]byte, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	finfo, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := finfo.Size()
	if size > maxSize {
		return nil, &fileSizeLimitError{size, maxSize}
	}
	buff := make([]byte, size)
	_, err = f.Read(buff)
	if err != nil {
		return nil, err
	}
	return buff, nil
}
