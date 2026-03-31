// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && test

package agentprovider

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/stretchr/testify/assert"
)

func TestNewConfigManagerDebugFromYAML(t *testing.T) {
	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
hostprofiler:
  debug:
    verbosity: detailed
`)
	mgr := newConfigManager(cfg)

	assert.Equal(t, "detailed", mgr.hostProfilerConfig.Debug["verbosity"])
}

func TestNewConfigManagerDebugFromEnvVar(t *testing.T) {
	t.Setenv("DD_HOSTPROFILER_DEBUG", `{"verbosity":"detailed"}`)

	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
`)
	mgr := newConfigManager(cfg)

	assert.Equal(t, "detailed", mgr.hostProfilerConfig.Debug["verbosity"])
}
