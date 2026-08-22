// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package workloadbalancinghelpers

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/stretchr/testify/assert"
)

func TestIsEnabled(t *testing.T) {
	cfg := config.NewMock(t)
	assert.False(t, IsEnabled(cfg))

	cfg.SetInTest("agent_workload_balancing.enabled", true)
	assert.True(t, IsEnabled(cfg))
}
