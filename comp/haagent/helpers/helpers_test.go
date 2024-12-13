// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package haagenthelpers

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/assert"
)

func TestIsEnabled(t *testing.T) {
	cfg := config.NewMock(t)
	assert.False(t, IsEnabled(cfg))

	cfg.Set("ha_agent.enabled", true, pkgconfigmodel.SourceUnknown)
	assert.True(t, IsEnabled(cfg))
}

func TestGetGroup(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.Set("ha_agent.group", "my-group", pkgconfigmodel.SourceUnknown)
	assert.Equal(t, "my-group", GetGroup(cfg))
}

func TestGetHaAgentTags(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.Set("ha_agent.group", "my-group", pkgconfigmodel.SourceUnknown)
	assert.Equal(t, []string{"agent_group:my-group"}, GetHaAgentTags(cfg))
}
