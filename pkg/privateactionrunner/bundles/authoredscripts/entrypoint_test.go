// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_authoredscripts

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthoredScriptsGetAction(t *testing.T) {
	bundle := NewAuthoredScripts(false)
	handler := bundle.GetAction("addRepo")

	assert.IsType(t, &RunAuthoredScriptHandler{}, handler)
	assert.Same(t, handler, bundle.GetAction("restartService"))
	assert.Nil(t, bundle.GetAction(""))
}

func TestRunAuthoredScriptDisabled(t *testing.T) {
	bundle := NewAuthoredScripts(false)

	output, err := bundle.GetAction("addRepo").Run(context.Background(), nil, nil)

	require.ErrorIs(t, err, errAuthoredScriptExecutionNotImplemented)
	assert.Nil(t, output)
}
