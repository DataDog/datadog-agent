// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package user

import (
	"errors"
	"os/user"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetGroupIDUnknownGroupError verifies that GetGroupID returns an error that
// matches user.UnknownGroupError (value type, not pointer) for a non-existent group.
// This is important because ensureGroup uses errors.As with the value type to suppress
// expected "group not found" warnings.
func TestGetGroupIDUnknownGroupError(t *testing.T) {
	_, err := user.LookupGroup("dd-agent-this-group-definitely-does-not-exist-12345")
	require.Error(t, err)

	var unknownGroupError user.UnknownGroupError
	assert.True(t, errors.As(err, &unknownGroupError), "error should match user.UnknownGroupError value type")

	var unknownGroupErrorPtr *user.UnknownGroupError
	assert.False(t, errors.As(err, &unknownGroupErrorPtr), "error should NOT match *user.UnknownGroupError pointer type (this was the original bug)")
}

// TestGetUserIDUnknownUserError verifies that user.Lookup returns an error that
// matches user.UnknownUserError (value type, not pointer) for a non-existent user.
func TestGetUserIDUnknownUserError(t *testing.T) {
	_, err := user.Lookup("dd-agent-this-user-definitely-does-not-exist-12345")
	require.Error(t, err)

	var unknownUserError user.UnknownUserError
	assert.True(t, errors.As(err, &unknownUserError), "error should match user.UnknownUserError value type")

	var unknownUserErrorPtr *user.UnknownUserError
	assert.False(t, errors.As(err, &unknownUserErrorPtr), "error should NOT match *user.UnknownUserError pointer type (this was the original bug)")
}
