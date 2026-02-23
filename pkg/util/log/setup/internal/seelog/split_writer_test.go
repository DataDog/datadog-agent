// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seelog

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitWriter_SuccessWriter(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	writer := newSplitWriter(&buf1, &buf2)
	n, err := writer.Write([]byte("test"))
	assert.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, "test", buf1.String())
	assert.Equal(t, "test", buf2.String())
}

func TestSplitWriter_FailingWriter(t *testing.T) {
	var buf bytes.Buffer
	err0 := errors.New("failing writer")
	failingWriter := newFailingWriter(0, err0)

	writer := newSplitWriter(failingWriter, &buf)
	n, err := writer.Write([]byte("test"))
	assert.ErrorIs(t, err, err0)
	assert.Equal(t, 0, n)
	assert.Equal(t, "test", buf.String())
}

func TestSplitWriter_FailingWriters(t *testing.T) {
	err1 := errors.New("failing writer1")
	failingWriter1 := newFailingWriter(1, err1)
	err2 := errors.New("failing writer2")
	failingWriter2 := newFailingWriter(2, err2)

	writer := newSplitWriter(failingWriter1, failingWriter2)
	n, err := writer.Write([]byte("test"))
	assert.ErrorIs(t, err, err1)
	assert.ErrorIs(t, err, err2)
	assert.Equal(t, 1, n)
}

func TestSingleWriter(t *testing.T) {
	var buf bytes.Buffer
	writer := newSplitWriter(&buf)
	assert.Same(t, writer, &buf)

	n, err := writer.Write([]byte("test"))
	assert.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, "test", buf.String())
}

type failingWriter struct {
	n   int
	err error
}

func newFailingWriter(n int, err error) io.Writer {
	return &failingWriter{n: n, err: err}
}

func (w *failingWriter) Write([]byte) (n int, err error) {
	return w.n, w.err
}
