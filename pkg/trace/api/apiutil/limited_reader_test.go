// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiutil

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

// fileMock simulates a file which can return both io.EOF and a byte count
// greater than 0.
type fileMock struct {
	data []byte
}

func newFileMock(data []byte) *fileMock {
	return &fileMock{data: data}
}

func (f *fileMock) Read(buf []byte) (n int, err error) {
	n = len(f.data)
	err = nil

	if n > cap(buf) {
		n = cap(buf)
	}

	if n == len(f.data) {
		err = io.EOF
	}

	copy(buf, f.data[:n])
	f.data = f.data[n:]

	return
}

func (f *fileMock) Close() error {
	f.data = nil
	return nil
}

func TestLimitedReader(t *testing.T) {
	buf := bytes.NewBufferString("foobar")
	r := io.NopCloser(buf)
	lr := NewLimitedReader(r, 3)

	tmp := make([]byte, 1)
	n, err := lr.Read(tmp)
	assert.Nil(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, []byte("f"), tmp)
	assert.Equal(t, int64(1), lr.Count)

	tmp = make([]byte, 4)
	n, err = lr.Read(tmp)
	assert.Nil(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, []byte("oo\x00\x00"), tmp)
	assert.Equal(t, int64(3), lr.Count)

	tmp = make([]byte, 1)
	_, err = lr.Read(tmp)
	assert.Equal(t, ErrLimitedReaderLimitReached, err)
	assert.Equal(t, int64(3), lr.Count)
}

func TestLimitedReaderEOFBuffer(t *testing.T) {
	buf := bytes.NewBufferString("foobar")
	r := io.NopCloser(buf)
	lr := NewLimitedReader(r, 12)

	tmp := make([]byte, 6)
	n, err := lr.Read(tmp)
	assert.Nil(t, err)
	assert.Equal(t, 6, n)
	assert.Equal(t, []byte("foobar"), tmp)
	assert.Equal(t, int64(6), lr.Count)

	tmp = make([]byte, 6)
	n, err = lr.Read(tmp)
	assert.Equal(t, io.EOF, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, int64(6), lr.Count)
}

func TestLimitedReaderEOFMockFile(t *testing.T) {
	file := newFileMock([]byte("foobar"))
	lr := NewLimitedReader(file, 12)

	tmp := make([]byte, 6)
	n, err := lr.Read(tmp)
	assert.Nil(t, err)
	assert.Equal(t, 6, n)
	assert.Equal(t, []byte("foobar"), tmp)
	assert.Equal(t, int64(6), lr.Count)

	tmp = make([]byte, 6)
	n, err = lr.Read(tmp)
	assert.Equal(t, io.EOF, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, int64(6), lr.Count)
}
