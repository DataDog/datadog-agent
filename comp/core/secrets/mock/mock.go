// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package mock offers a mock for the secrets Component allowing testing of secrets resolution.
package mock

import (
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets/utils"
)

// Mock is a mock of the secret Component useful for testing
type Mock struct {
	secretsCache   map[string]string
	callbacks      []secrets.SecretChangeCallback
	refreshHook    func() bool
	refreshNowHook func() (string, error)
}

var _ secrets.Component = (*Mock)(nil)

// New returns a MockResolver
func New(_ testing.TB) *Mock {
	return &Mock{}
}

// SetSecrets set the map of handle to secrets value for the mock
func (m *Mock) SetSecrets(secrets map[string]string) {
	m.secretsCache = secrets
}

// Configure is a noop for the mock
func (m *Mock) Configure(_ secrets.ConfigParams) {}

// Resolve resolves the secrets in the given yaml data by replacing secrets handles by their corresponding secret value
// from the data receive by `SetSecrets` method
func (m *Mock) Resolve(data []byte, origin string, _ string, _ string, notify bool) ([]byte, error) {
	var config interface{}
	err := yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("could not Unmarshal config: %s", err)
	}

	unknownSecrets := []string{}
	w := &utils.Walker{
		Resolver: func(path []string, value string) (string, error) {
			if ok, handle := utils.IsEnc(value); ok {
				if secretValue, ok := m.secretsCache[handle]; ok {
					// notify subscriptions
					if notify {
						for _, sub := range m.callbacks {
							sub(handle, origin, path, secretValue, secretValue)
						}
					}
					return secretValue, nil
				}
				unknownSecrets = append(unknownSecrets, handle)
			}
			return value, nil
		},
	}

	if err := w.Walk(&config); err != nil {
		return nil, err
	}

	if len(unknownSecrets) > 0 {
		return nil, fmt.Errorf("unknown secrets found: %s", strings.Join(unknownSecrets, ", "))
	}

	finalConfig, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("could not Marshal config after replacing encrypted secrets: %s", err)
	}
	return finalConfig, nil

}

// SubscribeToChanges registers a callback to be invoked whenever secrets are resolved or refreshed
func (m *Mock) SubscribeToChanges(callback secrets.SecretChangeCallback) {
	m.callbacks = append(m.callbacks, callback)
}

// SetRefreshHook sets a hook function that will be called when Refresh is invoked
func (m *Mock) SetRefreshHook(hook func() bool) {
	m.refreshHook = hook
}

// SetRefreshNowHook sets a hook function that will be called when RefreshNow is invoked
func (m *Mock) SetRefreshNowHook(hook func() (string, error)) {
	m.refreshNowHook = hook
}

// Refresh schedules an asynchronous secret refresh
func (m *Mock) Refresh() bool {
	if m.refreshHook != nil {
		return m.refreshHook()
	}
	return false
}

// RefreshNow performs an immediate blocking secret refresh
func (m *Mock) RefreshNow() (string, error) {
	if m.refreshNowHook != nil {
		return m.refreshNowHook()
	}
	return "", nil
}

// RemoveOrigin
func (m *Mock) RemoveOrigin(_ string) {}
