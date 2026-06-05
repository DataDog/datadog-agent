// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package configmock provides mocks for the config component.
package configmock

import (
	"testing"

	configdef "github.com/DataDog/datadog-agent/comp/core/config/def"
	configimpl "github.com/DataDog/datadog-agent/comp/core/config/impl"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
)

// New returns a mock for the config component.
func New(t testing.TB) configdef.Component {
	return configimpl.NewCfgFromPkgConfig(mock.New(t))
}

// NewWithOverrides creates a mock config and calls SetInTest on every item in overrides.
func NewWithOverrides(t testing.TB, overrides map[string]interface{}) configdef.Component {
	conf := mock.New(t)
	for k, v := range overrides {
		conf.SetInTest(k, v)
	}
	return configimpl.NewCfgFromPkgConfig(conf)
}

// NewFromYAML returns a mock for the config component with the given YAML content loaded into it.
func NewFromYAML(t testing.TB, yaml string) configdef.Component {
	return configimpl.NewCfgFromPkgConfig(mock.NewFromYAML(t, yaml))
}

// NewFromYAMLFile returns a mock for the config component with the given YAML file loaded into it.
func NewFromYAMLFile(t testing.TB, yamlFilePath string) configdef.Component {
	return configimpl.NewCfgFromPkgConfig(mock.NewFromFile(t, yamlFilePath))
}
