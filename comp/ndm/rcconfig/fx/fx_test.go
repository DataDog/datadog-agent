// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package fx

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	providertypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	ndmdiscovery "github.com/DataDog/datadog-agent/comp/ndmdiscovery/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
)

// fakeAdder records what was registered with autodiscovery.
type fakeAdder struct {
	added    []providertypes.ConfigProvider
	poll     []bool
	interval []time.Duration
}

func (a *fakeAdder) AddConfigProvider(p providertypes.ConfigProvider, poll bool, interval time.Duration) {
	a.added = append(a.added, p)
	a.poll = append(a.poll, poll)
	a.interval = append(a.interval, interval)
}

// fakeDiscovery is an ndmdiscovery.Component that accepts and records every range.
type fakeDiscovery struct {
	scheduled [][]ndmdiscovery.Range
}

func (d *fakeDiscovery) Schedule(ranges []ndmdiscovery.Range) map[string]error {
	d.scheduled = append(d.scheduled, ranges)
	return nil
}

func (d *fakeDiscovery) RangeCount() int { return 0 }

// fakeLifecycle collects the hooks a component registers.
type fakeLifecycle struct {
	hooks []fx.Hook
}

func (l *fakeLifecycle) Append(h fx.Hook) { l.hooks = append(l.hooks, h) }

func (l *fakeLifecycle) start(t *testing.T) {
	t.Helper()
	for _, h := range l.hooks {
		require.NoError(t, h.OnStart(context.Background()))
	}
}

const enabledYAML = `
remote_configuration:
  enabled: true
network_devices:
  remote_config:
    enabled: true
`

func TestNewListenerIsInertByDefault(t *testing.T) {
	adder := &fakeAdder{}

	listener, err := newListener(&fakeLifecycle{}, configmock.New(t), logmock.New(t), adder, &fakeDiscovery{})

	require.NoError(t, err)
	assert.Nil(t, listener.ListenerProvider, "a disabled component subscribes to nothing")
	assert.Empty(t, adder.added, "a disabled component registers no config provider")
}

func TestNewListenerIsInertWhenRemoteConfigurationIsOff(t *testing.T) {
	adder := &fakeAdder{}
	cfg := configmock.NewFromYAML(t, `
remote_configuration:
  enabled: false
network_devices:
  remote_config:
    enabled: true
`)

	listener, err := newListener(&fakeLifecycle{}, cfg, logmock.New(t), adder, &fakeDiscovery{})

	require.NoError(t, err)
	assert.Nil(t, listener.ListenerProvider)
	assert.Empty(t, adder.added)
}

func TestNewListenerIsInertWhenNDMRemoteConfigIsOff(t *testing.T) {
	adder := &fakeAdder{}
	cfg := configmock.NewFromYAML(t, `
remote_configuration:
  enabled: true
network_devices:
  remote_config:
    enabled: false
`)

	listener, err := newListener(&fakeLifecycle{}, cfg, logmock.New(t), adder, &fakeDiscovery{})

	require.NoError(t, err)
	assert.Nil(t, listener.ListenerProvider)
	assert.Empty(t, adder.added)
}

func TestNewListenerSubscribesToOneProductAndRegistersAStreamingProvider(t *testing.T) {
	adder := &fakeAdder{}

	listener, err := newListener(&fakeLifecycle{}, configmock.NewFromYAML(t, enabledYAML), logmock.New(t), adder, &fakeDiscovery{})

	require.NoError(t, err)
	require.Len(t, listener.ListenerProvider, 1, "exactly one product")
	assert.Contains(t, listener.ListenerProvider, data.ProductNDMConfig)

	require.Len(t, adder.added, 1)
	assert.Equal(t, "ndm-remote-config", adder.added[0].String())
	assert.False(t, adder.poll[0], "a streaming provider is not polled")
	assert.Zero(t, adder.interval[0])

	_, streaming := adder.added[0].(providertypes.StreamingConfigProvider)
	assert.True(t, streaming, "autodiscovery reads the stream instead of polling")
}

func TestNewListenerRegistersTheFeatureHandlers(t *testing.T) {
	adder := &fakeAdder{}

	_, err := newListener(&fakeLifecycle{}, configmock.NewFromYAML(t, enabledYAML), logmock.New(t), adder, &fakeDiscovery{})
	require.NoError(t, err)

	keys := adder.added[0].(interface{ RegisteredKeys() []string }).RegisteredKeys()
	assert.Equal(t, []string{"discovery", "snmp"}, keys)
}

func TestNewListenerAppliesTheDevelopmentConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ndm-dev.json")
	document, err := json.Marshal(map[string]any{
		"discovery": map[string]any{
			"ranges": []map[string]any{{
				"autodiscovery_id": "local-loopback",
				"namespace":        "default",
				"network_address":  "127.0.0.1/24",
				"interval_sec":     60,
			}},
		},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, document, 0o600))

	cfg := configmock.NewFromYAML(t, enabledYAML)
	cfg.SetInTest(devConfigKey, path)
	lc := &fakeLifecycle{}
	disco := &fakeDiscovery{}

	_, err = newListener(lc, cfg, logmock.New(t), &fakeAdder{}, disco)
	require.NoError(t, err)
	require.Empty(t, disco.scheduled, "nothing is applied before start")

	lc.start(t)

	require.Len(t, disco.scheduled, 1)
	require.Len(t, disco.scheduled[0], 1)
	assert.Equal(t, "local-loopback", disco.scheduled[0][0].ID)
	assert.Equal(t, "127.0.0.1/24", disco.scheduled[0][0].NetworkAddress)
}

func TestNewListenerRegistersNoHookWithoutADevelopmentConfig(t *testing.T) {
	lc := &fakeLifecycle{}

	_, err := newListener(lc, configmock.NewFromYAML(t, enabledYAML), logmock.New(t), &fakeAdder{}, &fakeDiscovery{})

	require.NoError(t, err)
	assert.Empty(t, lc.hooks)
}
