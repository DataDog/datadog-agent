// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package config

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
)

// NewMock returns a mock for the config component
func NewMock(t testing.TB) Component {
	return &cfg{Config: mock.New(t)}
}

// NewMockWithOverrides create a mock config and call SetWithoutSource on evey item in overrides
func NewMockWithOverrides(t testing.TB, overrides map[string]interface{}) Component {
	conf := mock.New(t)
	for k, v := range overrides {
		conf.SetWithoutSource(k, v)
	}
	return &cfg{Config: conf}
}

// NewMockFromYAML returns a mock for the config component with the given YAML content loaded into it.
func NewMockFromYAML(t testing.TB, yaml string) Component {
	return &cfg{Config: mock.NewFromYAML(t, yaml)}
}

// NewMockFromYAMLFile returns a mock for the config component with the given YAML file loaded into it.
func NewMockFromYAMLFile(t testing.TB, yamlFilePath string) Component {
	return &cfg{Config: mock.NewFromFile(t, yamlFilePath)}
}
