// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package configmock provides mocks for the config component.
package configmock

import (
	"testing"
	"time"

	configdef "github.com/DataDog/datadog-agent/comp/core/config/def"
	pkgmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// mockCfg wraps pkg/config/mock to implement config.Component without
// depending on the impl package.
type mockCfg struct {
	pkgconfigmodel.Config
}

func (m *mockCfg) Warnings() *pkgconfigmodel.Warnings { return &pkgconfigmodel.Warnings{} }
func (m *mockCfg) StartTime() time.Time               { return time.Time{} }

// New returns a mock for the config component.
func New(t *testing.T) configdef.Component {
	return &mockCfg{pkgmock.New(t)}
}

// NewWithTB returns a mock for the config component, accepting testing.TB for
// use in benchmarks and other contexts where *testing.T is not available.
func NewWithTB(t testing.TB) configdef.Component {
	return &mockCfg{pkgmock.New(t)}
}

// NewWithOverrides creates a mock config and calls SetInTest on every item in overrides.
func NewWithOverrides(t *testing.T, overrides map[string]interface{}) configdef.Component {
	conf := pkgmock.New(t)
	for k, v := range overrides {
		conf.SetInTest(k, v)
	}
	return &mockCfg{conf}
}

// NewWithOverridesTB creates a mock config accepting testing.TB, for use in
// benchmarks and helpers that receive testing.TB.
func NewWithOverridesTB(t testing.TB, overrides map[string]interface{}) configdef.Component {
	conf := pkgmock.New(t)
	for k, v := range overrides {
		conf.SetInTest(k, v)
	}
	return &mockCfg{conf}
}

// NewFromYAML returns a mock for the config component with the given YAML content loaded into it.
func NewFromYAML(t *testing.T, yaml string) configdef.Component {
	return &mockCfg{pkgmock.NewFromYAML(t, yaml)}
}

// NewFromYAMLTB returns a mock accepting testing.TB, for benchmarks and helpers.
func NewFromYAMLTB(t testing.TB, yaml string) configdef.Component {
	return &mockCfg{pkgmock.NewFromYAML(t, yaml)}
}

// NewFromYAMLFile returns a mock for the config component with the given YAML file loaded into it.
func NewFromYAMLFile(t *testing.T, yamlFilePath string) configdef.Component {
	return &mockCfg{pkgmock.NewFromFile(t, yamlFilePath)}
}

// NewFromYAMLFileTB returns a mock accepting testing.TB, for benchmarks and helpers.
func NewFromYAMLFileTB(t testing.TB, yamlFilePath string) configdef.Component {
	return &mockCfg{pkgmock.NewFromFile(t, yamlFilePath)}
}
