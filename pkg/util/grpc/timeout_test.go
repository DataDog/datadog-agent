// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDoWithTimeout_Success(t *testing.T) {
	err := DoWithTimeout(func() error {
		return nil
	}, 1*time.Second)

	assert.NoError(t, err)
}

func TestDoWithTimeout_FunctionError(t *testing.T) {
	expectedErr := errors.New("function error")
	err := DoWithTimeout(func() error {
		return expectedErr
	}, 1*time.Second)

	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestDoWithTimeout_Timeout(t *testing.T) {
	err := DoWithTimeout(func() error {
		time.Sleep(100 * time.Millisecond)
		return nil
	}, 10*time.Millisecond)

	assert.Error(t, err)
	s, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.DeadlineExceeded, s.Code())
}

func TestDoWithTimeout_FastFunction(t *testing.T) {
	called := false
	err := DoWithTimeout(func() error {
		called = true
		return nil
	}, 1*time.Second)

	assert.NoError(t, err)
	assert.True(t, called)
}
