// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package errors

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
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
	assert.Equal(t, taskErr, &InstallerError{
		err:  err,
		code: ErrDownloadFailed,
	})

	// Check that Wrap doesn't change anything if the error
	// is already an InstallerError
	taskErr2 := Wrap(ErrNotEnoughDiskSpace, taskErr)
	assert.Equal(t, taskErr2, &InstallerError{
		err:  err,
		code: ErrDownloadFailed,
	})

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
