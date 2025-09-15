// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discovery

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/stretchr/testify/assert"
)

func TestRegistry_RegisterConfig(t *testing.T) {
	registry := &Registry{
		discoveryMap: make(map[string]Info),
	}

	tests := []struct {
		name   string
		config integration.Config
		want   map[string]Info
	}{
		{
			name: "register config with discovery info",
			config: integration.Config{
				Name:            "postgres",
				ADIdentifiers:   []string{"postgres", "postgresql"},
				DiscoveryConfig: []byte(`{"log_source": "postgresql"}`),
			},
			want: map[string]Info{
				"postgres":   {LogSource: "postgresql"},
				"postgresql": {LogSource: "postgresql"},
			},
		},
		{
			name: "register config with YAML discovery info",
			config: integration.Config{
				Name:            "redis",
				ADIdentifiers:   []string{"redis"},
				DiscoveryConfig: []byte(`log_source: redis-server`),
			},
			want: map[string]Info{
				"redis": {LogSource: "redis-server"},
			},
		},
		{
			name: "register config without discovery info",
			config: integration.Config{
				Name:          "nginx",
				ADIdentifiers: []string{"nginx"},
			},
			want: map[string]Info{},
		},
		{
			name: "register config with empty discovery info",
			config: integration.Config{
				Name:            "mysql",
				ADIdentifiers:   []string{"mysql"},
				DiscoveryConfig: []byte(`{}`),
			},
			want: map[string]Info{
				"mysql": {LogSource: ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry.Reset()
			registry.RegisterConfig(tt.config)
			assert.Equal(t, tt.want, registry.discoveryMap)
		})
	}
}

func TestRegistry_GetLogSource(t *testing.T) {
	registry := &Registry{
		discoveryMap: map[string]Info{
			"postgres": {LogSource: "postgresql"},
			"redis":    {LogSource: "redis-server"},
			"empty":    {LogSource: ""},
		},
	}

	tests := []struct {
		name       string
		identifier string
		want       string
		wantExists bool
	}{
		{
			name:       "get existing log source",
			identifier: "postgres",
			want:       "postgresql",
			wantExists: true,
		},
		{
			name:       "get another existing log source",
			identifier: "redis",
			want:       "redis-server",
			wantExists: true,
		},
		{
			name:       "get non-existent identifier",
			identifier: "nginx",
			want:       "",
			wantExists: false,
		},
		{
			name:       "get identifier with empty log source",
			identifier: "empty",
			want:       "",
			wantExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, exists := registry.GetLogSource(tt.identifier)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantExists, exists)
		})
	}
}

func TestRegistry_Reset(t *testing.T) {
	registry := &Registry{
		discoveryMap: map[string]Info{
			"postgres": {LogSource: "postgresql"},
			"redis":    {LogSource: "redis-server"},
		},
	}

	registry.Reset()
	assert.Empty(t, registry.discoveryMap)
}

func TestGetRegistry_Singleton(t *testing.T) {
	reg1 := GetRegistry()
	reg2 := GetRegistry()
	assert.Same(t, reg1, reg2, "GetRegistry should return the same instance")
}
