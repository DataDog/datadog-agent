// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package command

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

type remoteConfigHandler struct {
	called *bool
}

func (h remoteConfigHandler) UpdateRemoteConfig(map[string]state.RawConfig, func(string, state.ApplyStatus)) {
	*h.called = true
}

var _ npcollector.RemoteConfigHandler = remoteConfigHandler{}

func TestNewNetworkPathRCListener(t *testing.T) {
	tests := []struct {
		name               string
		remoteConfig       bool
		networkPathConfig  bool
		expectedSubscribed bool
	}{
		{
			name: "both disabled",
		},
		{
			name:              "only global remote config enabled",
			remoteConfig:      true,
			networkPathConfig: false,
		},
		{
			name:               "only network path remote config enabled",
			remoteConfig:       false,
			networkPathConfig:  true,
			expectedSubscribed: false,
		},
		{
			name:               "both enabled",
			remoteConfig:       true,
			networkPathConfig:  true,
			expectedSubscribed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("remote_configuration.enabled", tt.remoteConfig)
			cfg.SetInTest("network_path.remote_config.enabled", tt.networkPathConfig)

			called := false
			listener := newNetworkPathRCListener(cfg, remoteConfigHandler{called: &called})
			callback, subscribed := listener.ListenerProvider[data.ProductNetworkPath]
			assert.Equal(t, tt.expectedSubscribed, subscribed)
			if subscribed {
				callback(nil, nil)
				assert.True(t, called)
			}
		})
	}
}
