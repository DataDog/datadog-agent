// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package errors

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCode(t *testing.T) {
	// Nil case
	assert.Equal(t, GetCode(nil), errUnknown)

	// Simple case
	var err error = &InstallerError{
		err:  errors.New("test: test"),
		code: ErrDownloadFailed,
	}
	assert.Equal(t, GetCode(err), ErrDownloadFailed)

	// Wrap
	err = fmt.Errorf("test1: %w", &InstallerError{
		err:  errors.New("test2: test3"),
		code: ErrDownloadFailed,
	})
	assert.Equal(t, GetCode(err), ErrDownloadFailed)

	// Multiple wraps
	err = fmt.Errorf("Wrap 2: %w", fmt.Errorf("Wrap 1: %w", err))
	assert.Equal(t, GetCode(err), ErrDownloadFailed)
}

func TestWrap(t *testing.T) {
	err := errors.New("test: test")
	taskErr := Wrap(ErrDownloadFailed, err)
	ie, ok := taskErr.(*InstallerError)
	require.True(t, ok)
	assert.Equal(t, err, ie.err)
	assert.Equal(t, ErrDownloadFailed, ie.code)

	// Check that Wrap doesn't change anything if the error
	// is already an InstallerError
	taskErr2 := Wrap(ErrNotEnoughDiskSpace, taskErr)
	assert.Equal(t, taskErr, taskErr2)

	taskErr3 := Wrap(ErrFilesystemIssue, fmt.Errorf("Wrap 2: %w", fmt.Errorf("Wrap 1: %w", taskErr2)))
	unwrapped := &InstallerError{}
	assert.True(t, errors.As(taskErr3, &unwrapped))
	assert.Equal(t, unwrapped.code, ErrDownloadFailed)
}

func TestToJSON(t *testing.T) {
	err := fmt.Errorf("test: %w", &InstallerError{
		err:  errors.New("test2: test3"),
		code: ErrDownloadFailed,
	})
	assert.Equal(t, ToJSON(err), `{"error":"test: test2: test3","code":1}`)
}

func TestFromJSON(t *testing.T) {
	json := `{"error":"test: test2: test3","code":1}`
	err := FromJSON(json)
	assert.Equal(t, err.Error(), "test: test2: test3")
	assert.Equal(t, GetCode(err), ErrDownloadFailed)
}

func TestWrapCapturesStack(t *testing.T) {
	err := errors.New("test error")
	wrapped := Wrap(ErrDownloadFailed, err)

	ie, ok := wrapped.(*InstallerError)
	require.True(t, ok)
	require.NotEmpty(t, ie.StackTrace(), "Wrap should capture a stack trace")

	assert.True(t, stackContainsFunc(ie.StackTrace(), "TestWrapCapturesStack"),
		"Stack trace should contain the calling test function")
}

func TestUnwrap(t *testing.T) {
	inner := errors.New("inner error")
	wrapped := Wrap(ErrDownloadFailed, inner)

	ie, ok := wrapped.(*InstallerError)
	require.True(t, ok)
	assert.Equal(t, inner, ie.Unwrap())

	// errors.Unwrap should work
	assert.Equal(t, inner, errors.Unwrap(wrapped))
}

func TestWithStack(t *testing.T) {
	assert.Nil(t, WithStack(nil))

	err := errors.New("plain error")
	withStack := WithStack(err)
	require.NotNil(t, withStack)
	assert.Equal(t, "plain error", withStack.Error())

	se, ok := withStack.(*stackError)
	require.True(t, ok)
	require.NotEmpty(t, se.StackTrace())
	assert.Equal(t, err, se.Unwrap())

	assert.True(t, stackContainsFunc(se.StackTrace(), "TestWithStack"),
		"Stack trace should contain the calling test function")
}

func TestWithStackNoDoubleWrap(t *testing.T) {
	err := errors.New("test")
	wrapped := Wrap(ErrDownloadFailed, err)

	result := WithStack(wrapped)
	assert.Equal(t, wrapped, result, "WithStack should return the original error if it already has a stack")
}

func stackContainsFunc(pcs []uintptr, name string) bool {
	frames := runtime.CallersFrames(pcs)
	for {
		frame, more := frames.Next()
		if strings.Contains(frame.Function, name) {
			return true
		}
		if !more {
			break
		}
	}
	return false
}
