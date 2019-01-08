// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

import (
	"errors"
	"io"
)

var readerNotInitializedError = errors.New("reader not initialized")

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
		err := readerNotInitializedError
		return 0, err
	}

	return s.reader.Read(p)
}

func (s *safeReader) Close() error {
	if s.reader == nil {
		return readerNotInitializedError
	}

	return s.reader.Close()
}
