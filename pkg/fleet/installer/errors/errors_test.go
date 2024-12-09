// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package errors

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFromErr(t *testing.T) {
	var err error = &InstallerError{
		err:  fmt.Errorf("test: test"),
		code: ErrDownloadFailed,
	}
	taskErr := FromErr(err)
	assert.Equal(t, taskErr, &InstallerError{
		err:  fmt.Errorf("test: test"),
		code: ErrDownloadFailed,
	})

	assert.Nil(t, FromErr(nil))
}

func TestFromErrWithWrap(t *testing.T) {
	err := fmt.Errorf("test: %w", &InstallerError{
		err:  fmt.Errorf("test: test"),
		code: ErrDownloadFailed,
	})
	taskErr := FromErr(err)
	assert.Equal(t, taskErr, &InstallerError{
		err:  fmt.Errorf("test: test"),
		code: ErrDownloadFailed,
	})

	taskErr2 := fmt.Errorf("Wrap 2: %w", fmt.Errorf("Wrap 1: %w", taskErr))
	assert.Equal(t, FromErr(taskErr2).Code(), ErrDownloadFailed)
	assert.Nil(t, FromErr(nil))
}

func TestWrap(t *testing.T) {
	err := fmt.Errorf("test: test")
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
	assert.Equal(t, FromErr(taskErr3).Code(), ErrDownloadFailed)
}
