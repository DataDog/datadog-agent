// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

const (
	testCoreSchema = `
product_dependencies:
  npm:
    dependencies: []
  no_npm:
    dependencies: []
properties: {}
`
	testSystemProbeSchema = `
properties:
  network_config:
    node_type: section
    properties:
      enabled:
        node_type: setting
        default: false
        product_defaults:
          npm: true
          no_npm: false
`
)

func useTestProductSchemas(t *testing.T) {
	pkgconfigsetup.SetProductEnablementSchemasForTest(t, testCoreSchema, testSystemProbeSchema)
}

// Products must be applied before the modules are computed from the configuration
func TestProductEnablementEnablesModules(t *testing.T) {
	useTestProductSchemas(t)
	_ = mock.NewSystemProbe(t)
	mock.New(t).SetInTest("products", []string{"npm"})

	cfg, err := New("", "")
	require.NoError(t, err)
	assert.True(t, cfg.ModuleIsEnabled(NetworkTracerModule))
	assert.Equal(t, pkgconfigmodel.SourceProductEnablement, pkgconfigsetup.SystemProbe().GetSource("network_config.enabled"))
}

func TestProductEnablementConflict(t *testing.T) {
	useTestProductSchemas(t)
	_ = mock.NewSystemProbe(t)
	mock.New(t).SetInTest("products", []string{"npm", "no_npm"})

	_, err := New("", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network_config.enabled: no_npm=false, npm=true")
}
