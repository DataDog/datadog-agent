// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package com_datadoghq_authoredscripts

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunAuthoredScriptEnabled(t *testing.T) {
	handler := NewRunAuthoredScriptHandler(true)

	_, err := handler.Run(context.Background(), nil, nil)

	require.Error(t, err)
	assert.NotErrorIs(t, err, errAuthoredScriptExecutionNotImplemented)
	assert.ErrorContains(t, err, "authored-script task is required")
}
