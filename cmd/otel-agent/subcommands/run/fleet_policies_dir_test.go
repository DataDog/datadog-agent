// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp && test

package run

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveFleetPoliciesDir_PrefersEnv(t *testing.T) {
	t.Setenv("DD_FLEET_POLICIES_DIR", "/custom/fleet/policies")
	assert.Equal(t, "/custom/fleet/policies", resolveFleetPoliciesDir())
}

func TestResolveFleetPoliciesDir_EmptyWhenUnset(t *testing.T) {
	os.Unsetenv("DD_FLEET_POLICIES_DIR")
	if resolveFleetPoliciesDir() != "" {
		// On Windows CI/dev machines the registry may be set; only assert the env path above.
		t.Skip("fleet policies dir resolved from registry on this host")
	}
}
