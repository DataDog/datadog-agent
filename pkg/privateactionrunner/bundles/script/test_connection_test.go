// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package com_datadoghq_script

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectionInvalidConfigurationReturnsGenericError(t *testing.T) {
	output, err := NewTestConnectionHandler().Run(context.Background(), nil, scriptCredentials("sensitive-scalar: ["))

	require.NoError(t, err)
	result := output.(*TestConnectionOutputs)
	assert.False(t, result.ConfigurationValid)
	assert.Equal(t, []string{"Failed to parse script configuration"}, result.Errors)
	assert.NotContains(t, result.Errors[0], "sensitive-scalar")
}

func TestConnectionValidConfigurationRemainsUnchanged(t *testing.T) {
	credentials := scriptCredentials(`schemaId: script-credentials-v1
runPredefinedScript:
  hello:
    command: [echo, hello]
`)

	output, err := NewTestConnectionHandler().Run(context.Background(), nil, credentials)

	require.NoError(t, err)
	result := output.(*TestConnectionOutputs)
	assert.True(t, result.ConfigurationValid)
	assert.Empty(t, result.Errors)
	assert.Equal(t, []string{"echo", "hello"}, result.AvailableScripts["hello"].Command)
}
