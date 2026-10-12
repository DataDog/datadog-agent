// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package rcserviceimpl

import (
	"testing"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/stretchr/testify/require"
)

func TestLocallyDisabledServiceIsAbsent(t *testing.T) {
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{"remote_configuration.enabled": false})
	result := NewComponent(Dependencies{Cfg: cfg})
	_, present := result.Comp.Get()
	require.False(t, present, "local disablement is absence, not an observed org denial")
}
