// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"errors"
	"io"
)

var errReaderNotInitialized = errors.New("reader not initialized")

// safeReader wraps an io.ReadCloser in such a way that a nil reader is
// treated as a recoverable error and does not cause a panic.  This is a
// belt-and-suspenders way to avoid such nil-pointer panics; see
// https://github.com/DataDog/datadog-agent/pull/2817
type safeReader struct {
	reader io.ReadCloser
}

func newSafeReader() *safeReader {
	return &safeReader{}
}

func (s *safeReader) setUnsafeReader(reader io.ReadCloser) {
	s.reader = reader
}

func (s *safeReader) Read(p []byte) (int, error) {
	if s.reader == nil {
		err := errReaderNotInitialized
		return 0, err
	}

	return s.reader.Read(p)
}

func (s *safeReader) Close() error {
	if s.reader == nil {
		return errReaderNotInitialized
	}

	return s.reader.Close()
}
