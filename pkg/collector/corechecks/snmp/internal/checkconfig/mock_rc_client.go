// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package checkconfig

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// MockRCClient mocks rcclient.Component
type MockRCClient struct {
	subscribed bool
	err        error
	profiles   map[string]state.RawConfig
}

// MakeMockClient creates a MockRCClient
func MakeMockClient(profiles []profiledefinition.ProfileDefinition) (*MockRCClient, error) {
	update := make(map[string]state.RawConfig)
	for _, profile := range profiles {
		bytes, err := json.Marshal(profiledefinition.DeviceProfileRcConfig{Profile: profile})
		if err != nil {
			return nil, err
		}
		update[profile.Name] = state.RawConfig{
			Config: bytes,
		}
	}
	return &MockRCClient{
		subscribed: false,
		err:        nil,
		profiles:   update,
	}, nil
}

// SubscribeAgentTask subscribe the remote-config client to AGENT_TASK
func (m *MockRCClient) SubscribeAgentTask() {}

// noop
func (m *MockRCClient) applyStateCallback(string, state.ApplyStatus) {}

// Subscribe starts listening to a specific product update
func (m *MockRCClient) Subscribe(product data.Product, fn func(update map[string]state.RawConfig,
	applyStateCallback func(string, state.ApplyStatus))) {
	if product != state.ProductNDMDeviceProfilesCustom {
		m.err = fmt.Errorf("unexpected subscription to %v", product)
		return
	}
	if m.subscribed {
		m.err = fmt.Errorf("double subscription to ProductNDMDeviceProfilesCustom")
		return
	}
	m.subscribed = true
	fn(m.profiles, m.applyStateCallback)
}
