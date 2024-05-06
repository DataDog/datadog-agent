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

func TestFrom(t *testing.T) {
	var err error = &InstallerError{
		err:  fmt.Errorf("test: test"),
		code: ErrDownloadFailed,
	}
	taskErr := From(err)
	assert.Equal(t, taskErr, &InstallerError{
		err:  fmt.Errorf("test: test"),
		code: ErrDownloadFailed,
	})

	assert.Nil(t, From(nil))
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
	taskErr2 := Wrap(ErrInstallFailed, taskErr)
	assert.Equal(t, taskErr2, &InstallerError{
		err:  err,
		code: ErrDownloadFailed,
	})
}
