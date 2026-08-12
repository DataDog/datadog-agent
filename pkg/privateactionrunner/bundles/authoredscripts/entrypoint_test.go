// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_authoredscripts

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuthoredScriptsGetAction(t *testing.T) {
	bundle := NewAuthoredScripts()
	handler := bundle.GetAction("addRepo")

	assert.IsType(t, &RunAuthoredScriptHandler{}, handler)
	assert.Same(t, handler, bundle.GetAction("restartService"))
	assert.Nil(t, bundle.GetAction(""))
}
