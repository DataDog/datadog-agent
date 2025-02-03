// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestConfigIgnoredComms(t *testing.T) {
	tests := []struct {
		name  string   // The name of the test.
		comms []string // list of command names to test
	}{
		{
			name:  "empty list of commands",
			comms: []string{},
		},
		{
			name: "short commands in config list",
			comms: []string{
				"cron",
				"polkitd",
				"rsyslogd",
				"bash",
				"sshd",
			},
		},
		{
			name: "long commands in config list",
			comms: []string{
				"containerd-shim-runc-v2",
				"calico-node",
				"unattended-upgrade-shutdown",
				"bash",
				"kube-controller-manager",
			},
		},
	}

	// test with custom command names of different lengths
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockSystemProbe := mock.NewSystemProbe(t)
			require.NotEmpty(t, mockSystemProbe)

			commsStr := strings.Join(test.comms, "   ") // intentionally multiple spaces for sensitivity testing
			mockSystemProbe.SetWithoutSource("discovery.ignored_command_names", commsStr)

			discovery := newDiscovery(nil, nil)
			require.NotEmpty(t, discovery)

			require.Equal(t, len(discovery.config.ignoreComms), len(test.comms))

			for _, cmd := range test.comms {
				if len(cmd) > maxCommLen {
					cmd = cmd[:maxCommLen]
				}
				_, found := discovery.config.ignoreComms[cmd]
				assert.True(t, found)
			}
		})
	}

	t.Run("check default config length", func(t *testing.T) {
		mock.NewSystemProbe(t)
		discovery := newDiscovery(nil, nil)
		require.NotEmpty(t, discovery)

		assert.Equal(t, len(discovery.config.ignoreComms), 10)
	})

	t.Run("check command names in env variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_DISCOVERY_IGNORED_COMMAND_NAMES", "dummy1 dummy2")

		discovery := newDiscovery(nil, nil)
		require.NotEmpty(t, discovery)

		_, found := discovery.config.ignoreComms["dummy1"]
		assert.True(t, found)
		_, found = discovery.config.ignoreComms["dummy2"]
		assert.True(t, found)
	})
}

func TestConfigIgnoredServices(t *testing.T) {
	tests := []struct {
		name     string   // the name of the test.
		services []string // list of services to test
	}{
		{
			name:     "empty list of services",
			services: []string{},
		},
		{
			name: "non-empty list of services",
			services: []string{
				"datadog-agent",
				"another-agent",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockSystemProbe := mock.NewSystemProbe(t)
			require.NotEmpty(t, mockSystemProbe)

			servicesStr := strings.Join(test.services, "   ") // intentionally multiple spaces for sensitivity testing
			mockSystemProbe.SetWithoutSource("discovery.ignored_services", servicesStr)

			discovery := newDiscovery(nil, nil)
			require.NotEmpty(t, discovery)

			require.Equal(t, len(discovery.config.ignoreServices), len(test.services))

			for _, service := range test.services {
				_, found := discovery.config.ignoreServices[service]
				assert.True(t, found)
			}
		})
	}

	t.Run("check default number of services", func(t *testing.T) {
		mock.NewSystemProbe(t)
		discovery := newDiscovery(nil, nil)
		require.NotEmpty(t, discovery)

		assert.Equal(t, len(discovery.config.ignoreServices), 6)
	})

	t.Run("check services in env variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_DISCOVERY_IGNORED_SERVICES", "service1 service2")

		discovery := newDiscovery(nil, nil)
		require.NotEmpty(t, discovery)

		_, found := discovery.config.ignoreServices["service1"]
		assert.True(t, found)
		_, found = discovery.config.ignoreServices["service2"]
		assert.True(t, found)
	})
}
