// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

type mockSecretScenario struct {
	expectedData   []byte
	expectedOrigin string
	returnedData   []byte
	returnedError  error
	called         int
}

type MockSecretResolver struct {
	t           *testing.T
	scenarios   []mockSecretScenario
	subscribers []secrets.SecretChangeCallback
}

var _ secrets.Component = (*MockSecretResolver)(nil)

func (m *MockSecretResolver) Configure(_ secrets.ConfigParams) {}

func (m *MockSecretResolver) Resolve(data []byte, origin string, _ string, _ string, _ bool) ([]byte, error) {
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

func (m *MockSecretResolver) RemoveOrigin(_ string) {}

func (m *MockSecretResolver) SubscribeToChanges(callback secrets.SecretChangeCallback) {
	if m.subscribers == nil {
		m.subscribers = make([]secrets.SecretChangeCallback, 0)
	}
	m.subscribers = append(m.subscribers, callback)
}

func (m *MockSecretResolver) Refresh() bool {
	return false
}

func (m *MockSecretResolver) RefreshNow() (string, error) {
	return "", nil
}

func (m *MockSecretResolver) IsValueFromSecret(_ string) bool {
	return false
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

// nolint: deadcode, unused
func (m *MockSecretResolver) triggerCallback(handle, origin string, path []string, oldValue, newValue any) {
	for _, subscriber := range m.subscribers {
		subscriber(handle, origin, path, oldValue, newValue)
	}
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

func makeScenariosForConfig(conf integration.Config) []mockSecretScenario {
	digest := conf.Digest()
	return []mockSecretScenario{
		{
			expectedData:   []byte("param1: ENC[foo]"),
			expectedOrigin: digest,
			returnedData:   []byte("param1: foo"),
			returnedError:  nil,
		},
		{
			expectedData:   []byte("param2: ENC[bar]"),
			expectedOrigin: digest,
			returnedData:   []byte("param2: bar"),
			returnedError:  nil,
		},
		{
			expectedData:   []byte("param3: ENC[met]"),
			expectedOrigin: digest,
			returnedData:   []byte("param3: met"),
			returnedError:  nil,
		},
		{
			expectedData:   []byte("param4: ENC[log]"),
			expectedOrigin: digest,
			returnedData:   []byte("param4: log"),
			returnedError:  nil,
		},
	}
}

var makeSharedScenarios = func() []mockSecretScenario {
	return makeScenariosForConfig(sharedTpl)
}

func TestSecretResolve(t *testing.T) {
	mockResolve := &MockSecretResolver{t: t, scenarios: makeSharedScenarios()}
	newConfig, err := decryptConfig(sharedTpl, mockResolve, sharedTpl.Digest())
	require.NoError(t, err)

	assert.NotEqual(t, newConfig.Instances, sharedTpl.Instances)

	assert.True(t, mockResolve.haveAllScenariosBeenCalled())
}

// TestDecryptConfigInstanceFailureSkipsInstance verifies that when one instance fails to
// resolve secrets, only that instance is excluded — the other instance and the remaining
// sections (metrics, logs) are still resolved and scheduled.
func TestDecryptConfigInstanceFailureSkipsInstance(t *testing.T) {
	tpl := integration.Config{
		Name:          "cpu",
		ADIdentifiers: []string{"redis"},
		InitConfig:    []byte("param1: ENC[foo]"),
		Instances: []integration.Data{
			[]byte("bad: ENC[unknown]"),
			[]byte("good: ENC[bar]"),
		},
		MetricConfig: []byte("param3: ENC[met]"),
		LogsConfig:   []byte("param4: ENC[log]"),
	}
	mockResolve := &MockSecretResolver{
		t: t,
		scenarios: []mockSecretScenario{
			{expectedData: []byte("param1: ENC[foo]"), expectedOrigin: tpl.Digest(), returnedData: []byte("param1: foo")},
			{expectedData: []byte("bad: ENC[unknown]"), expectedOrigin: tpl.Digest(), returnedData: []byte("bad: ENC[unknown]"), returnedError: errors.New("unknown handle")},
			{expectedData: []byte("good: ENC[bar]"), expectedOrigin: tpl.Digest(), returnedData: []byte("good: bar")},
			{expectedData: []byte("param3: ENC[met]"), expectedOrigin: tpl.Digest(), returnedData: []byte("param3: met")},
			{expectedData: []byte("param4: ENC[log]"), expectedOrigin: tpl.Digest(), returnedData: []byte("param4: log")},
		},
	}

	newConfig, err := decryptConfig(tpl, mockResolve, tpl.Digest())

	// error is propagated so the caller knows an instance was dropped
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown handle")

	// the failing instance is excluded; the surviving instance is present
	require.Len(t, newConfig.Instances, 1)
	assert.Equal(t, integration.Data("good: bar"), newConfig.Instances[0])

	// init_config, metrics, and logs are fully resolved
	assert.Equal(t, integration.Data("param1: foo"), newConfig.InitConfig)
	assert.Equal(t, integration.Data("param3: met"), newConfig.MetricConfig)
	assert.Equal(t, integration.Data("param4: log"), newConfig.LogsConfig)

	assert.True(t, mockResolve.haveAllScenariosBeenCalled())
}

// TestDecryptConfigInitConfigFailureDropsAll verifies that a failure in init_config drops
// the entire config — no instances are resolved since init_config is shared by all.
func TestDecryptConfigInitConfigFailureDropsAll(t *testing.T) {
	mockResolve := &MockSecretResolver{
		t: t,
		scenarios: []mockSecretScenario{
			{
				expectedData:   []byte("param1: ENC[foo]"),
				expectedOrigin: sharedTpl.Digest(),
				returnedData:   []byte("param1: ENC[foo]"),
				returnedError:  errors.New("could not resolve secret handle(s)"),
			},
		},
	}

	newConfig, err := decryptConfig(sharedTpl, mockResolve, sharedTpl.Digest())

	// error propagated, instances cleared so caller drops the entire config
	require.Error(t, err)
	assert.Contains(t, err.Error(), "init_config")
	assert.Empty(t, newConfig.Instances)
	assert.True(t, mockResolve.haveAllScenariosBeenCalled())
}

func TestSkipSecretResolve(t *testing.T) {
	mockResolve := &MockSecretResolver{t: t, scenarios: makeSharedScenarios()}

	cfg := configmock.New(t)
	cfg.SetWithoutSource("secret_backend_skip_checks", true)
	defer cfg.SetWithoutSource("secret_backend_skip_checks", false)

	c, err := decryptConfig(sharedTpl, mockResolve, sharedTpl.Digest())
	require.NoError(t, err)

	assert.Equal(t, sharedTpl.Instances, c.Instances)
	assert.Equal(t, sharedTpl.InitConfig, c.InitConfig)

	assert.True(t, mockResolve.haveAllScenariosNotCalled())
}
