// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_script

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLimitedWriter_UnderLimit(t *testing.T) {
	stdout, stderr := newLimitedStdoutStderrWritersPair(100)

	n, err := stdout.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	n, err = stderr.Write([]byte("world"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	assert.Equal(t, "hello", stdout.String())
	assert.Equal(t, "world", stderr.String())
	assert.False(t, stdout.LimitReached())
	assert.False(t, stderr.LimitReached())
}

func TestLimitedWriter_ExactLimit(t *testing.T) {
	stdout, stderr := newLimitedStdoutStderrWritersPair(10)

	n, err := stdout.Write([]byte("12345"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	n, err = stderr.Write([]byte("67890"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	assert.Equal(t, "12345", stdout.String())
	assert.Equal(t, "67890", stderr.String())
	assert.False(t, stdout.LimitReached())
	assert.False(t, stderr.LimitReached())
}

func TestLimitedWriter_ExceedsLimit(t *testing.T) {
	stdout, stderr := newLimitedStdoutStderrWritersPair(10)

	n, err := stdout.Write([]byte("12345678"))
	require.NoError(t, err)
	assert.Equal(t, 8, n)

	// This write exceeds the combined limit of 10; only 2 bytes should be written.
	n, err = stderr.Write([]byte("abcdef"))
	assert.ErrorIs(t, err, errOutputLimitExceeded)
	assert.Equal(t, 2, n)

	assert.Equal(t, "12345678", stdout.String())
	assert.Equal(t, "ab", stderr.String())
	assert.True(t, stderr.LimitReached())
}

func TestLimitedWriter_SubsequentWritesAfterLimit(t *testing.T) {
	stdout, stderr := newLimitedStdoutStderrWritersPair(5)

	_, err := stdout.Write([]byte("12345"))
	require.NoError(t, err)

	// Limit reached on next write
	n, err := stderr.Write([]byte("x"))
	assert.ErrorIs(t, err, errOutputLimitExceeded)
	assert.Equal(t, 0, n)
	assert.True(t, stderr.LimitReached())

	// Further writes to stdout also fail
	n, err = stdout.Write([]byte("y"))
	assert.ErrorIs(t, err, errOutputLimitExceeded)
	assert.Equal(t, 0, n)
	assert.True(t, stdout.LimitReached())
}

func TestLimitedWriter_SingleWriterExceedsLimit(t *testing.T) {
	stdout, stderr := newLimitedStdoutStderrWritersPair(5)

	n, err := stdout.Write([]byte("0123456789"))
	assert.ErrorIs(t, err, errOutputLimitExceeded)
	assert.Equal(t, 5, n)
	assert.Equal(t, "01234", stdout.String())
	assert.True(t, stdout.LimitReached())

	// stderr should also be blocked by the shared counter
	n, err = stderr.Write([]byte("a"))
	assert.ErrorIs(t, err, errOutputLimitExceeded)
	assert.Equal(t, 0, n)
}

func TestLimitedWriter_SharedCounter(t *testing.T) {
	stdout, stderr := newLimitedStdoutStderrWritersPair(10)

	// Alternate writes between stdout and stderr
	stdout.Write([]byte("aa"))  // shared = 2
	stderr.Write([]byte("bbb")) // shared = 5
	stdout.Write([]byte("cc"))  // shared = 7

	// 3 bytes remaining in the shared budget
	n, err := stderr.Write([]byte("dddd"))
	assert.ErrorIs(t, err, errOutputLimitExceeded)
	assert.Equal(t, 3, n)

	assert.Equal(t, "aacc", stdout.String())
	assert.Equal(t, "bbbddd", stderr.String())
}

func TestLimitedWriter_ConcurrentWrites(t *testing.T) {
	const limit int64 = 1024
	stdout, stderr := newLimitedStdoutStderrWritersPair(limit)

	chunk := []byte("abcdefghij") // 10 bytes per write
	var wg sync.WaitGroup

	// Simulate exec.Cmd: two goroutines writing concurrently
	wg.Add(2)
	go func() {
		defer wg.Done()
		for {
			if _, err := stdout.Write(chunk); err != nil {
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for {
			if _, err := stderr.Write(chunk); err != nil {
				return
			}
		}
	}()
	wg.Wait()

	total := int64(stdout.Len() + stderr.Len())
	assert.True(t, stdout.LimitReached() || stderr.LimitReached())
	assert.LessOrEqual(t, total, limit+int64(len(chunk)),
		"total bytes (%d) should not exceed limit (%d) by more than one chunk (%d)", total, limit, len(chunk))
}
