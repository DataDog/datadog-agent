// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package errors

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNotFound(t *testing.T) {
	// New
	err := NewNotFound("foo")
	require.Error(t, err)
	require.Equal(t, `"foo" not found`, err.Error())

	// Is
	require.True(t, IsNotFound(err))
	require.False(t, IsNotFound(fmt.Errorf("fake")))
	require.False(t, IsNotFound(fmt.Errorf(`"foo" not found`)))
}

func TestRetriable(t *testing.T) {
	// New
	err := NewRetriable("foo", errors.New("bar"))
	require.Error(t, err)
	require.Equal(t, `couldn't fetch "foo": bar`, err.Error())

	// Is
	var errFunc = func() error { return NewRetriable("foo", errors.New("bar")) }
	require.True(t, IsRetriable(errFunc()))
	require.False(t, IsRetriable(fmt.Errorf("fake")))
	require.False(t, IsRetriable(fmt.Errorf(`couldn't fetch "foo": bar`)))
}

func TestRemoteService(t *testing.T) {
	// New
	err := NewRemoteServiceError("datadog cluster agent", "500 Internal Server Error")
	require.Error(t, err)
	require.Equal(t, `"datadog cluster agent" is unavailable: 500 Internal Server Error`, err.Error())

	// Is
	require.True(t, IsRemoteService(err))
	require.False(t, IsRemoteService(errors.New("fake")))
	require.False(t, IsRemoteService(errors.New(`"datadog cluster agent" is unavailable: 500 Internal Server Error`)))
}

func TestTimeout(t *testing.T) {
	// New
	err := NewTimeoutError("datadog cluster agent", errors.New("context deadline exceeded"))
	require.Error(t, err)
	require.Equal(t, `timeout calling "datadog cluster agent": context deadline exceeded`, err.Error())

	// Is
	require.True(t, IsTimeout(err))
	require.False(t, IsTimeout(errors.New("fake")))
	require.False(t, IsTimeout(errors.New(`timeout calling "datadog cluster agent": context deadline exceeded`)))
}
