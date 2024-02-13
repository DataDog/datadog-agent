// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscovery

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
)

type mockSecretScenario struct {
	expectedData   []byte
	expectedOrigin string
	returnedData   []byte
	returnedError  error
	called         int
}

type MockSecretResolver struct {
	t         *testing.T
	scenarios []mockSecretScenario
}

var _ secrets.Component = (*MockSecretResolver)(nil)

func (m *MockSecretResolver) Configure(_ secrets.ConfigParams) {}

func (m *MockSecretResolver) GetDebugInfo(_ io.Writer) {}

func (m *MockSecretResolver) Resolve(data []byte, origin string) ([]byte, error) {
	if m.scenarios == nil {
		return data, nil
	}
	for n, scenario := range m.scenarios {
		if bytes.Equal(data, scenario.expectedData) && origin == scenario.expectedOrigin {
			m.scenarios[n].called++
			return scenario.returnedData, scenario.returnedError
		}
	}
	m.t.Errorf("Resolve called with unexpected arguments: data=%s, origin=%s", string(data), origin)
	return nil, fmt.Errorf("Resolve called with unexpected arguments: data=%s, origin=%s", string(data), origin)
}

func (m *MockSecretResolver) SubscribeToChanges(_ secrets.SecretChangeCallback) {
}

func (m *MockSecretResolver) Refresh() (string, error) {
	return "", nil
}

func (m *MockSecretResolver) haveAllScenariosBeenCalled() bool {
	for _, scenario := range m.scenarios {
		if scenario.called == 0 {
			fmt.Printf("%#v\n", m.scenarios)
			return false
		}
	}
	return true
}

func (m *MockSecretResolver) haveAllScenariosNotCalled() bool {
	for _, scenario := range m.scenarios {
		if scenario.called != 0 {
			return false
		}
	}
	return true
}

var sharedTpl = integration.Config{
	Name:          "cpu",
	ADIdentifiers: []string{"redis"},
	InitConfig:    []byte("param1: ENC[foo]"),
	Instances: []integration.Data{
		[]byte("param2: ENC[bar]"),
	},
	MetricConfig: []byte("param3: ENC[met]"),
	LogsConfig:   []byte("param4: ENC[log]"),
}

var makeSharedScenarios = func() []mockSecretScenario {
	return []mockSecretScenario{
		{
			expectedData:   []byte("param1: ENC[foo]"),
			expectedOrigin: "cpu",
			returnedData:   []byte("param1: foo"),
			returnedError:  nil,
		},
		{
			expectedData:   []byte("param2: ENC[bar]"),
			expectedOrigin: "cpu",
			returnedData:   []byte("param2: bar"),
			returnedError:  nil,
		},
		{
			expectedData:   []byte("param3: ENC[met]"),
			expectedOrigin: "cpu",
			returnedData:   []byte("param3: met"),
			returnedError:  nil,
		},
		{
			expectedData:   []byte("param4: ENC[log]"),
			expectedOrigin: "cpu",
			returnedData:   []byte("param4: log"),
			returnedError:  nil,
		},
	}
}

func TestSecretResolve(t *testing.T) {
	mockResolve := &MockSecretResolver{t, makeSharedScenarios()}

	newConfig, err := decryptConfig(sharedTpl, mockResolve)
	require.NoError(t, err)

	assert.NotEqual(t, newConfig.Instances, sharedTpl.Instances)

	assert.True(t, mockResolve.haveAllScenariosBeenCalled())
}

func TestSkipSecretResolve(t *testing.T) {
	mockResolve := &MockSecretResolver{t, makeSharedScenarios()}

	cfg := config.Mock(t)
	cfg.SetWithoutSource("secret_backend_skip_checks", true)
	defer cfg.SetWithoutSource("secret_backend_skip_checks", false)

	c, err := decryptConfig(sharedTpl, mockResolve)
	require.NoError(t, err)

	assert.Equal(t, sharedTpl.Instances, c.Instances)
	assert.Equal(t, sharedTpl.InitConfig, c.InitConfig)

	assert.True(t, mockResolve.haveAllScenariosNotCalled())
}
