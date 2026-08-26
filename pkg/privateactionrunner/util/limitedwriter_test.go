// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package util

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLimitedWriter_WriteUnderLimit(t *testing.T) {
	stdout, _ := NewLimitedStdoutStderrWritersPair(100)

	n, err := stdout.Write([]byte("hello"))

	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", stdout.String())
	assert.False(t, stdout.LimitReached())
}

func TestLimitedWriter_WriteExactlyAtLimit(t *testing.T) {
	stdout, _ := NewLimitedStdoutStderrWritersPair(5)

	n, err := stdout.Write([]byte("hello"))

	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.False(t, stdout.LimitReached())
}

func TestLimitedWriter_WriteExceedsLimit(t *testing.T) {
	stdout, _ := NewLimitedStdoutStderrWritersPair(5)

	n, err := stdout.Write([]byte("hello world"))

	require.ErrorIs(t, err, ErrOutputLimitExceeded)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", stdout.String())
	assert.True(t, stdout.LimitReached())
}

func TestLimitedWriter_StickyAfterLimitReached(t *testing.T) {
	stdout, _ := NewLimitedStdoutStderrWritersPair(5)

	_, err := stdout.Write([]byte("hello world"))
	require.ErrorIs(t, err, ErrOutputLimitExceeded)

	n, err := stdout.Write([]byte("more"))

	require.ErrorIs(t, err, ErrOutputLimitExceeded)
	assert.Equal(t, 0, n)
	assert.Equal(t, "hello", stdout.String())
}

func TestNewLimitedStdoutStderrWritersPair_SharesLimitAcrossWriters(t *testing.T) {
	stdout, stderr := NewLimitedStdoutStderrWritersPair(10)

	n, err := stdout.Write([]byte("12345"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	n, err = stderr.Write([]byte("12345"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	n, err = stdout.Write([]byte("x"))
	require.ErrorIs(t, err, ErrOutputLimitExceeded)
	assert.Equal(t, 0, n)
	assert.True(t, stdout.LimitReached())
	assert.False(t, stderr.LimitReached())
}

func TestLimitedWriter_Len(t *testing.T) {
	stdout, _ := NewLimitedStdoutStderrWritersPair(100)

	_, err := stdout.Write([]byte("hello"))

	require.NoError(t, err)
	assert.Equal(t, 5, stdout.Len())
}

func TestErrOutputLimitExceeded_IsSentinel(t *testing.T) {
	assert.True(t, errors.Is(ErrOutputLimitExceeded, ErrOutputLimitExceeded))
}
