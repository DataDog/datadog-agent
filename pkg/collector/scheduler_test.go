// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

type MockCheck struct {
	core.CheckBase
	Name       string
	LoaderName string
}

func (m MockCheck) Run() error {
	// not used in test
	panic("implement me")
}

func (m MockCheck) Loader() string {
	return m.LoaderName
}

func (m MockCheck) String() string {
	return fmt.Sprintf("Loader: %s, Check: %s", m.LoaderName, m.Name)
}

type MockCoreLoader struct{}

func (l *MockCoreLoader) Name() string {
	return "core"
}

// Load loads a check
func (l *MockCoreLoader) Load(_ sender.SenderManager, config integration.Config, _ integration.Data) (check.Check, error) {
	mockCheck := MockCheck{Name: config.Name, LoaderName: l.Name()}
	return &mockCheck, nil
}

type MockPythonLoader struct{}

func (l *MockPythonLoader) Name() string {
	return "python"
}

// Load loads a check
func (l *MockPythonLoader) Load(_ sender.SenderManager, config integration.Config, _ integration.Data) (check.Check, error) {
	mockCheck := MockCheck{Name: config.Name, LoaderName: l.Name()}
	return &mockCheck, nil
}

func TestAddLoader(t *testing.T) {
	s := CheckScheduler{}
	assert.Len(t, s.loaders, 0)
	s.addLoader(&MockCoreLoader{})
	s.addLoader(&MockCoreLoader{}) // noop
	assert.Len(t, s.loaders, 1)
}

func TestGetChecksFromConfigs(t *testing.T) {
	s := CheckScheduler{}
	assert.Len(t, s.loaders, 0)
	s.addLoader(&MockCoreLoader{})
	s.addLoader(&MockPythonLoader{})

	// test instance level loader selection
	conf1 := integration.Config{
		Name: "check_a",
		Instances: []integration.Data{
			integration.Data("{\"loader\": \"python\"}"),
			integration.Data("{\"loader\": \"core\"}"),
			integration.Data("{\"loader\": \"wrong\"}"),
			integration.Data("{}"), // default to init config loader
		},
		InitConfig: integration.Data("{\"loader\": \"core\"}"),
	}
	// test init config level loader selection
	conf2 := integration.Config{
		Name:       "check_b",
		Instances:  []integration.Data{integration.Data("{\"value\": 1}")},
		InitConfig: integration.Data("{\"loader\": \"python\"}"),
	}
	// test that wrong loader will be skipped
	conf3 := integration.Config{
		Name:       "check_wrong",
		Instances:  []integration.Data{integration.Data("{\"value\": 1}")},
		InitConfig: integration.Data("{\"loader\": \"wrong_loader\"}"),
	}
	// test that first loader is selected when no loader is selected
	// this is the current behaviour
	conf4 := integration.Config{
		Name:       "check_c",
		Instances:  []integration.Data{integration.Data("{}")},
		InitConfig: integration.Data("{}"),
	}

	checks := s.GetChecksFromConfigs([]integration.Config{conf1, conf2, conf3, conf4}, false)

	assert.Len(t, s.loaders, 2)

	var actualChecks []string

	for _, c := range checks {
		actualChecks = append(actualChecks, c.String())
	}
	assert.Equal(t, []string{
		"Loader: python, Check: check_a",
		"Loader: core, Check: check_a",
		"Loader: core, Check: check_a",
		"Loader: python, Check: check_b",
		"Loader: core, Check: check_c",
	}, actualChecks)
}
