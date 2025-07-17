// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package networkdeviceconfigimpl

import (
	"fmt"
	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/stretchr/testify/assert"
	"testing"
)

type fakeFactory struct {
	client RemoteClient
}

func (f *fakeFactory) Connect(_ string, _ AuthCredentials) (RemoteClient, error) {
	return f.client, nil
}

func NewFakeFactory(c RemoteClient) RemoteClientFactory {
	return &fakeFactory{client: c}
}

// FakeRemoteClient is a "mocked" RemoteClient to help with testing
type FakeRemoteClient struct {
	Session *FakeRemoteSession
	Closed  bool
}

func (f *FakeRemoteClient) NewSession() (RemoteSession, error) {
	return f.Session, nil
}

func (f *FakeRemoteClient) Close() error {
	f.Closed = true
	return nil
}

// FakeRemoteSession simulates a RemoteSession
type FakeRemoteSession struct {
	OutputMap map[string]string // cmd -> output
	Closed    bool
	Calls     []string
}

func (f *FakeRemoteSession) CombinedOutput(cmd string) ([]byte, error) {
	f.Calls = append(f.Calls, cmd)

	if output, ok := f.OutputMap[cmd]; ok {
		return []byte(output), nil
	}
	return []byte(""), fmt.Errorf("no such command: %s", cmd)
}

func (f *FakeRemoteSession) Close() error {
	f.Closed = true
	return nil
}

func NewTestComponent(reqs Requires, factory RemoteClientFactory) (Provides, error) {
	ncmConfig, err := newConfig(reqs.Config)
	if err != nil {
		return Provides{}, reqs.Logger.Errorf("Failed to read network device configuration: %v", err)
	}
	var impl = &networkDeviceConfigImpl{
		config:        ncmConfig,
		log:           reqs.Logger,
		clientFactory: factory,
	}
	provides := Provides{
		Comp: impl,
	}
	return provides, nil
}

func TestRetrieveConfiguration_Real(t *testing.T) {
	var tests = []struct {
		name           string
		deviceIP       string
		configYaml     string
		commandsMap    map[string]string
		expectedOutput string
	}{
		{
			name:     "Retrieve running configuration from device",
			deviceIP: "10.0.0.1",
			configYaml: `
network_device_config_management:
  namespace: test
  devices:
    - ip_address: 10.0.0.1
      auth:
        username: admin
        password: password
        port: 22
        protocol: tcp
`,
			commandsMap: map[string]string{
				`show running-config`: "interface GigabitEthernet0/0\n ip address",
			},
			expectedOutput: "interface GigabitEthernet0/0\n ip address",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configComponent := config.NewMockFromYAML(t, tt.configYaml)
			logComponent := logmock.New(t)
			mockFactory := NewFakeFactory(&FakeRemoteClient{
				Session: &FakeRemoteSession{
					OutputMap: tt.commandsMap,
				},
			})

			requires := Requires{
				Config: configComponent,
				Logger: logComponent,
			}

			provides, _ := NewTestComponent(requires, mockFactory)
			component := provides.Comp

			actual, err := component.RetrieveRunningConfig(tt.deviceIP)
			assert.Nil(t, err)
			assert.Equal(t, tt.expectedOutput, actual)
		})
	}
}
