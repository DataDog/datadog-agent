// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build docker

package docker

import (
	"errors"
	"io"
	"time"
)

var errReaderNotInitialized = errors.New("reader not initialized")

const defaultBackoffDuration = time.Second
const maxBackoffDuration = 30 * time.Second

type safeReader struct {
	reader io.ReadCloser

	err error

	backoffRetry           int
	backoffWaitDuration    time.Duration
	backoffDefaultDuration time.Duration
}

func newSafeReader() *safeReader {
	return &safeReader{
		backoffDefaultDuration: defaultBackoffDuration,
	}
}

func (s *safeReader) Success() {
	s.err = nil
	s.backoffRetry = 0
	s.backoffWaitDuration = 0
}

func (s *safeReader) getBackoffAndIncrement() time.Duration {
	if s.backoffWaitDuration == maxBackoffDuration {
		return s.backoffWaitDuration
	}
	duration := s.backoffWaitDuration
	s.backoffRetry++
	s.backoffWaitDuration += time.Duration(s.backoffRetry) * s.backoffDefaultDuration
	if s.backoffWaitDuration > maxBackoffDuration {
		s.backoffWaitDuration = maxBackoffDuration
	}

	return duration
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
