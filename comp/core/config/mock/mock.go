// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package mock

import (
	"testing"

	config "github.com/DataDog/datadog-agent/comp/core/config/def"
	configimpl "github.com/DataDog/datadog-agent/comp/core/config/impl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	setup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

type Provides struct {
	Comp config.Component
}

type cfg struct {
	model.Config
}

func (c *cfg) Warnings() *model.Warnings {
	return nil
}

type mockDependencies struct {
	Params config.MockParams
}

func (m mockDependencies) getParams() *config.Params {
	p := m.Params.Params
	return &p
}

func (m mockDependencies) getSecretResolver() (secrets.Component, bool) {
	return nil, false
}

// newMock exported mock builder to allow modifying mocks that might be
// supplied in tests and used for dep injection.
func newMock(deps mockDependencies, t testing.TB) (config.Component, error) {
	var mockConf model.Config

	if deps.Params.ConfFilePath != "" {
		mockConf = mock.NewFromFile(t, deps.Params.ConfFilePath)
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

// New returns a mock for the config component
func New(t testing.TB) Provides {
	return Provides{
		Comp: &cfg{Config: mock.New(t)},
	}
}

// NewMockFromYAML returns a mock for the config component with the given YAML content loaded into it.
func NewMockFromYAML(t testing.TB, yaml string) config.Component {
	return &cfg{Config: mock.NewFromYAML(t, yaml)}
}

// NewMockFromYAMLFile returns a mock for the config component with the given YAML file loaded into it.
func NewMockFromYAMLFile(t testing.TB, yamlFilePath string) config.Component {
	return &cfg{Config: mock.NewFromFile(t, yamlFilePath)}
}

// NewComponent returns a mock for the config component with custom mock params.
func NewComponent(deps configimpl.Requires, t testing.TB) (configimpl.Provides, error) {
	mockDeps := mockDependencies{
		Params: config.MockParams{
			Params: deps.Params,
		},
	}
	comp, err := newMock(mockDeps, t)
	return configimpl.Provides{
		Comp: comp,
	}, err
}
