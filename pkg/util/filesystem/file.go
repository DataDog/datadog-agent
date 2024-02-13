// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filesystem

import (
	"bufio"
	"fmt"
	"os"
)

// FileExists returns true if a file exists and is accessible, false otherwise
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// FileSizeLimitError is the error returned when a file's size is larger than the limit
type FileSizeLimitError struct {
	Size  int64
	Limit int64
}

// Error returns a string describing the file size limit error
func (e *FileSizeLimitError) Error() string {
	return fmt.Sprintf("file larger than max size: %d > %d", e.Size, e.Limit)
}

// ReadFileWithSizeLimit reads a file's contents unless it is larger than the size limit
func ReadFileWithSizeLimit(filename string, maxSize int64) ([]byte, error) {
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
		return nil, &FileSizeLimitError{size, maxSize}
	}
	buff := make([]byte, size)
	_, err = f.Read(buff)
	if err != nil {
		return nil, err
	}
	return buff, nil
}

// ReadLines reads a file line by line
func ReadLines(filename string) ([]string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return []string{""}, err
	}
	defer f.Close()

	var ret []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		ret = append(ret, scanner.Text())
	}
	return ret, scanner.Err()
}
