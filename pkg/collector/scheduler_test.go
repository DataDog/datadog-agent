// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package collector

import (
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/stretchr/testify/assert"
)
type MockCheck struct {
	core.CheckBase
	Name string
}

func (m MockCheck) Run() error {
	// not used in test
	panic("implement me")
}

func (m MockCheck) String() string {
	return m.Name
}

type MockLoader struct{}

func (l *MockLoader) Name() string {
	return "mock_loader"
}

func (l *MockLoader) Load(config integration.Config, instance integration.Data) (check.Check, error) {
	mockCheck := MockCheck{Name: config.Name}
	return &mockCheck, nil
}

type MockLoader2 struct{}

func (l *MockLoader2) Name() string {
	return "mock_loader2"
}

func (l *MockLoader2) Load(config integration.Config, instance integration.Data) (check.Check, error) {
	return nil, nil
}

func TestAddLoader(t *testing.T) {
	s := CheckScheduler{}
	assert.Len(t, s.loaders, 0)
	s.AddLoader(&MockLoader{})
	s.AddLoader(&MockLoader{}) // noop
	assert.Len(t, s.loaders, 1)
}


func TestGetChecksFromConfigs(t *testing.T) {
	s := CheckScheduler{}
	assert.Len(t, s.loaders, 0)
	s.AddLoader(&MockLoader{})
	s.AddLoader(&MockLoader2{})

	conf1 := integration.Config{
		Name:       "check1",
		Instances:  []integration.Data{integration.Data("{\"value\": 1}")},
		InitConfig: integration.Data("{\"loader\": \"mock_loader\"}"),
	}
	conf2 := integration.Config{
		Name:       "check2",
		Instances:  []integration.Data{integration.Data("{\"value\": 1}")},
		InitConfig: integration.Data("{\"loader\": \"mock_loader\"}"),
	}
	conf3 := integration.Config{
		Name:       "check3",
		Instances:  []integration.Data{integration.Data("{\"value\": 1}")},
		InitConfig: integration.Data("{\"loader\": \"wrong_loader\"}"),
	}

	configs := s.GetChecksFromConfigs([]integration.Config{conf1, conf1, conf2, conf3}, false)

	assert.Len(t, s.loaders, 2)
	assert.Len(t, configs, 3)

	assert.Equal(t, configs[0].String(), "check1")
	assert.Equal(t, configs[1].String(), "check1")
	assert.Equal(t, configs[2].String(), "check2")
}
