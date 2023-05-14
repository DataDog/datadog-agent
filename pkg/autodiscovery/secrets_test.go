// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscovery

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

type MockSecretDecrypt struct {
	t         *testing.T
	scenarios []mockSecretScenario
}

func (m *MockSecretDecrypt) getDecryptFunc() func([]byte, string) ([]byte, error) {
	return func(data []byte, origin string) ([]byte, error) {
		for n, scenario := range m.scenarios {
			if bytes.Compare(data, scenario.expectedData) == 0 && origin == scenario.expectedOrigin {
				m.scenarios[n].called++
				return scenario.returnedData, scenario.returnedError
			}
		}
		m.t.Errorf("Decrypt called with unexpected arguments: data=%s, origin=%s", data, origin)
		return nil, fmt.Errorf("Decrypt called with unexpected arguments: data=%s, origin=%s", data, origin)
	}
}

func (m *MockSecretDecrypt) haveAllScenariosBeenCalled() bool {
	for _, scenario := range m.scenarios {
		if scenario.called == 0 {
			fmt.Printf("%#v\n", m.scenarios)
			return false
		}
	}
	return true
}

func (m *MockSecretDecrypt) haveAllScenariosNotCalled() bool {
	for _, scenario := range m.scenarios {
		if scenario.called != 0 {
			return false
		}
	}
	return true
}

// Install this secret decryptor, and return a function to uninstall it
func (m *MockSecretDecrypt) install() func() {
	originalSecretsDecrypt := secretsDecrypt
	secretsDecrypt = m.getDecryptFunc()
	return func() { secretsDecrypt = originalSecretsDecrypt }
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

func TestSecretDecrypt(t *testing.T) {
	mockDecrypt := MockSecretDecrypt{t, makeSharedScenarios()}
	defer mockDecrypt.install()()

	newConfig, err := decryptConfig(sharedTpl)
	require.NoError(t, err)

	assert.NotEqual(t, newConfig.Instances, sharedTpl.Instances)

	assert.True(t, mockDecrypt.haveAllScenariosBeenCalled())
}

func TestSkipSecretDecrypt(t *testing.T) {
	mockDecrypt := MockSecretDecrypt{t, makeSharedScenarios()}
	defer mockDecrypt.install()()

	cfg := config.Mock(t)
	cfg.Set("secret_backend_skip_checks", true)
	defer cfg.Set("secret_backend_skip_checks", false)

	c, err := decryptConfig(sharedTpl)
	require.NoError(t, err)

	assert.Equal(t, sharedTpl.Instances, c.Instances)
	assert.Equal(t, sharedTpl.InitConfig, c.InitConfig)

	assert.True(t, mockDecrypt.haveAllScenariosNotCalled())
}
