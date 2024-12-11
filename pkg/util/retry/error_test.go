// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package retry

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorOK(t *testing.T) {
	var err error = &Error{
		LogicError:    errors.New("logic error"),
		RessourceName: "mocked",
		RetryStatus:   PermaFail,
	}

	ok, retryErr := IsRetryError(err)
	assert.True(t, ok)
	assert.Equal(t, PermaFail, retryErr.RetryStatus)
	assert.True(t, IsErrPermaFail(err))
}

func TestIsErrWillRetry(t *testing.T) {
	var err error = &Error{
		LogicError:    errors.New("logic error"),
		RessourceName: "mocked",
		RetryStatus:   FailWillRetry,
	}
	assert.True(t, IsErrWillRetry(err))
}

func TestErrorNOK(t *testing.T) {
	err := errors.New("dumb error")
	ok, retryErr := IsRetryError(err)
	assert.False(t, ok)
	assert.Nil(t, retryErr)
}
