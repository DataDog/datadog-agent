// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test
// +build test

package config

import (
	"testing"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	setup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

type mockDependencies struct {
	fx.In

	Params MockParams
}

func (m mockDependencies) getParams() *Params {
	p := m.Params.Params
	return &p
}

func (m mockDependencies) getSecretResolver() (secrets.Component, bool) {
	return nil, false
}

// newMock exported mock builder to allow modifying mocks that might be
// supplied in tests and used for dep injection.
func newMock(deps mockDependencies, t testing.TB) (Component, error) {
	var mockConf model.Config

	if deps.Params.ConfFilePath != "" {
		mockConf = mock.NewFromFile(deps.Params.ConfFilePath)
	} else {
		mockConf = mock.New(t)
	}

	// Overrides are explicit and will take precedence over any other setting
	for k, v := range deps.Params.Overrides {
		mockConf.SetWithoutSource(k, v)
	}

	setup.LoadProxyFromEnv(mockConf)
	return &cfg{Config: mockConf}, nil
}

// NewMock returns a mock for the config component
func NewMock(t testing.TB) Component {
	return &cfg{Config: mock.New(t)}
}

// NewMockFromYAML returns a mock for the config component with the given YAML content loaded into it.
func NewMockFromYAML(t testing.TB, yaml string) Component {
	return &cfg{Config: mock.NewFromYAML(t, yaml)}
}

// NewMockFromYAMLFile returns a mock for the config component with the given YAML file loaded into it.
func NewMockFromYAMLFile(t testing.TB, yamlFilePath string) Component {
	return &cfg{Config: mock.NewFromFile(t, yamlFilePath)}
}
