// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

var errFoo = errors.New("foo error")
var errBar = errors.New("bar error")

type ReadCloserMock struct {
	io.Reader
	closer func() error
}

func (r *ReadCloserMock) Close() error {
	return r.closer()
}

func NewReadCloserMock(r io.Reader, closer func() error) io.ReadCloser {
	return &ReadCloserMock{
		Reader: r,
		closer: closer,
	}
}

type ReadErrorMock struct {
	io.Reader
}

func (r *ReadErrorMock) Read(p []byte) (int, error) {
	return 0, errFoo
}

func TestSafeReaderRead(t *testing.T) {
	reader := newSafeReader()
	bytesArray := []byte("foo")
	mockReadCloserNoError := NewReadCloserMock(bytes.NewReader(bytesArray), func() error {
		return nil
	})
	mockReadCloserReadError := NewReadCloserMock(&ReadErrorMock{}, func() error {
		return nil
	})

	n, err := reader.Read(bytesArray)
	assert.Equal(t, 0, n)
	assert.Equal(t, errReaderNotInitialized, err)

	reader.setUnsafeReader(mockReadCloserNoError)
	n, err = reader.Read(bytesArray)
	assert.Equal(t, len(bytesArray), n)
	assert.Nil(t, err)

	reader.setUnsafeReader(mockReadCloserReadError)
	n, err = reader.Read(bytesArray)
	assert.Equal(t, 0, n)
	assert.Equal(t, errFoo, err)

	reader.setUnsafeReader(nil)
	n, err = reader.Read(bytesArray)
	assert.Equal(t, 0, n)
	assert.Equal(t, errReaderNotInitialized, err)
}

func TestSafeReaderClose(t *testing.T) {
	reader := newSafeReader()
	mockReadCloserNoError := NewReadCloserMock(&ReadErrorMock{}, func() error {
		return nil
	})
	mockReadCloserCloseError := NewReadCloserMock(&ReadErrorMock{}, func() error {
		return errBar
	})

	err := reader.Close()
	assert.Equal(t, errReaderNotInitialized, err)

	reader.setUnsafeReader(mockReadCloserNoError)
	err = reader.Close()
	assert.Nil(t, err)

	reader.setUnsafeReader(mockReadCloserCloseError)
	err = reader.Close()
	assert.Equal(t, errBar, err)

	reader.setUnsafeReader(nil)
	err = reader.Close()
	assert.Equal(t, errReaderNotInitialized, err)
}
