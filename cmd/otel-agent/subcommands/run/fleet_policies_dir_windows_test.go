// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp && test && windows

package run

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Stable fleet policies layout under the managed configs root when registry is unset.
const expectedStableFleetPoliciesRel = "datadog-agent/stable"

func TestResolveFleetPoliciesDir_FallsBackToStableFleetPoliciesDirWhenUnset(t *testing.T) {
	t.Setenv("DD_FLEET_POLICIES_DIR", "")
	dir := resolveFleetPoliciesDir()
	assert.NotEmpty(t, dir)
	normalized := filepath.ToSlash(filepath.Clean(dir))
	assert.True(t, strings.HasSuffix(normalized, expectedStableFleetPoliciesRel), normalized)
}
