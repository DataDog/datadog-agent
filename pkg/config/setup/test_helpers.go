// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package setup

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

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
	conf := newEmptyMockConf(t)
	InitConfig(conf)
	conf.SetConfigFile("")
	pkgconfigmodel.ApplyOverrideFuncs(conf)
	return conf
}

// newSchemaTestConf returns a config initialized with the full Agent schema and
// the schema built, with dynamic schema left disabled. Unlike newTestConf, it
// mirrors production: setting an unknown key is rejected instead of silently
// accepted, so it can catch settings that the code writes but never registered.
func newSchemaTestConf(t *testing.T, yamlConfig string) pkgconfigmodel.BuildableConfig {
	cfg := create.NewConfig("test")
	InitConfig(cfg)
	cfg.BuildSchema()
	cfg.SetConfigType("yaml")
	require.NoError(t, cfg.ReadConfig(strings.NewReader(yamlConfig)))
	return cfg
}
