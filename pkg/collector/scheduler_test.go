// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/option"
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
	return m.Name
}

type MockCoreLoader struct{}

func (l *MockCoreLoader) Name() string {
	return "core"
}

// Load loads a check
func (l *MockCoreLoader) Load(_ sender.SenderManager, config integration.Config, _ integration.Data, _ int) (check.Check, error) {
	mockCheck := MockCheck{Name: config.Name, LoaderName: l.Name()}
	return &mockCheck, nil
}

type MockPythonLoader struct{}

func (l *MockPythonLoader) Name() string {
	return "python"
}

// Load loads a check
func (l *MockPythonLoader) Load(_ sender.SenderManager, config integration.Config, _ integration.Data, _ int) (check.Check, error) {
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
		"check_a",
		"check_a",
		"check_a",
		"check_b",
		"check_c",
	}, actualChecks)
}

// MockCollector is a mock implementation of collector.Component for testing
type MockCollector struct {
	RunCheckCalls []check.Check // Track which checks were run
	RunCheckError error         // Error to return from RunCheck
}

func (m *MockCollector) RunCheck(c check.Check) (checkid.ID, error) {
	m.RunCheckCalls = append(m.RunCheckCalls, c)
	if m.RunCheckError != nil {
		return "", m.RunCheckError
	}
	return c.ID(), nil
}

func (m *MockCollector) StopCheck(_ checkid.ID) error {
	return nil
}

func (m *MockCollector) ReloadAllCheckInstances(_ string, _ []check.Check) ([]checkid.ID, error) {
	return nil, nil
}

func (m *MockCollector) GetChecks() []check.Check {
	return nil
}

func (m *MockCollector) MapOverChecks(cb func([]check.Info)) {
	cb(nil)
}

func (m *MockCollector) AddEventReceiver(_ collector.EventReceiver) {
}

func TestSchedule_AllChecksAllowed(t *testing.T) {
	// Test that when not in basic mode, all checks are scheduled
	mockCollector := &MockCollector{}
	s := &CheckScheduler{
		collector:      option.New[collector.Component](mockCollector),
		configToChecks: make(map[string][]checkid.ID),
	}
	s.addLoader(&MockCoreLoader{})

	configs := []integration.Config{
		{
			Name:       "cpu",
			Instances:  []integration.Data{integration.Data("{}")},
			InitConfig: integration.Data("{}"),
		},
		{
			Name:       "disk",
			Instances:  []integration.Data{integration.Data("{}")},
			InitConfig: integration.Data("{}"),
		},
		{
			Name:       "custom_check", // Test custom_.* pattern
			Instances:  []integration.Data{integration.Data("{}")},
			InitConfig: integration.Data("{}"),
		},
	}

	s.Schedule(configs)

	// All checks should be run when not in basic mode
	assert.Len(t, mockCollector.RunCheckCalls, len(configs))
	for i, c := range configs {
		assert.Equal(t, c.Name, mockCollector.RunCheckCalls[i].(*MockCheck).Name)
	}
}
