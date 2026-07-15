// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp && test

package run

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveFleetPoliciesDir_PrefersEnv(t *testing.T) {
	t.Setenv("DD_FLEET_POLICIES_DIR", "/custom/fleet/policies")
	assert.Equal(t, "/custom/fleet/policies", resolveFleetPoliciesDir())
}
