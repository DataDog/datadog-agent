// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && test

package setup

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const expectedStableFleetPoliciesRel = "datadog-agent/stable"

func TestFleetConfigOverride_FallsBackToStableFleetPoliciesDirWhenUnset(t *testing.T) {
	t.Setenv("DD_FLEET_POLICIES_DIR", "")

	config := newTestConf(t)
	FleetConfigOverride(config)

	dir := config.GetString("fleet_policies_dir")
	assert.NotEmpty(t, dir)
	normalized := filepath.ToSlash(filepath.Clean(dir))
	assert.True(t, strings.HasSuffix(normalized, expectedStableFleetPoliciesRel), normalized)
}

func TestFleetConfigOverride_RespectsEnvOverride(t *testing.T) {
	const customDir = `C:\custom\fleet\policies`
	t.Setenv("DD_FLEET_POLICIES_DIR", customDir)

	config := newTestConf(t)
	FleetConfigOverride(config)

	assert.Equal(t, customDir, config.GetString("fleet_policies_dir"))
}
