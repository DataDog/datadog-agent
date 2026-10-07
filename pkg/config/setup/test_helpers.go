// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package setup

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/create"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// newEmptyMockConf returns an empty config appropriate for running tests
// we can't use pkg/config/mock here because that package depends upon this one, so
// this avoids a circular dependency
func newEmptyMockConf(_ *testing.T) pkgconfigmodel.BuildableConfig {
	cfg := create.NewConfig("test")
	cfg.SetTestOnlyDynamicSchema(true)
	return cfg
}

// newTestConf generates and returns a new configuration that has been setup
// by running the schema constructing code InitConfig found in setup/config.go
func newTestConf(t *testing.T) pkgconfigmodel.BuildableConfig {
	conf := newTestConfWithoutOverrides(t)
	pkgconfigmodel.ApplyOverrideFuncs(conf)
	return conf
}

// newTestConfWithoutOverrides is newTestConf without ApplyOverrideFuncs, for tests that need to run a
// single override func a controlled number of times. The override funcs are a global append-only slice,
// so ApplyOverrideFuncs runs each of them an order-dependent number of times across the suite.
func newTestConfWithoutOverrides(t *testing.T) pkgconfigmodel.BuildableConfig {
	conf := newEmptyMockConf(t)
	InitConfig(conf)
	conf.SetConfigFile("")
	return conf
}

// SetProductEnablementSchemasForTest replaces the core and system-probe schemas used by product enablement for the
// duration of the test.
func SetProductEnablementSchemasForTest(t testing.TB, core, systemProbe string) {
	previousCore, previousSystemProbe := coreSchemaGetter, systemProbeSchemaGetter
	coreSchemaGetter = func() ([]byte, error) { return []byte(core), nil }
	systemProbeSchemaGetter = func() ([]byte, error) { return []byte(systemProbe), nil }
	t.Cleanup(func() { coreSchemaGetter, systemProbeSchemaGetter = previousCore, previousSystemProbe })
}
