// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package fx

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// fakeDiscovery is an ndmdiscovery.Component that accepts every range.
type fakeDiscovery struct{}

func (fakeDiscovery) Schedule(_ []ndmdiscovery.Range) map[string]error { return nil }

func (fakeDiscovery) RangeCount() int { return 0 }

const enabledYAML = `
remote_configuration:
  enabled: true
network_devices:
  remote_config:
    enabled: true
`

func TestNewListenerIsInertByDefault(t *testing.T) {
	adder := &fakeAdder{}

	listener, err := newListener(configmock.New(t), logmock.New(t), adder, fakeDiscovery{})

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

	listener, err := newListener(cfg, logmock.New(t), adder, fakeDiscovery{})

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

	listener, err := newListener(cfg, logmock.New(t), adder, fakeDiscovery{})

	require.NoError(t, err)
	assert.Nil(t, listener.ListenerProvider)
	assert.Empty(t, adder.added)
}

func TestNewListenerSubscribesToOneProductAndRegistersAStreamingProvider(t *testing.T) {
	adder := &fakeAdder{}

	listener, err := newListener(configmock.NewFromYAML(t, enabledYAML), logmock.New(t), adder, fakeDiscovery{})

	require.NoError(t, err)
	require.Len(t, listener.ListenerProvider, 1, "exactly one product")
	assert.Contains(t, listener.ListenerProvider, data.ProductManagedDeploymentsDebug)

	require.Len(t, adder.added, 1)
	assert.Equal(t, "ndm-remote-config", adder.added[0].String())
	assert.False(t, adder.poll[0], "a streaming provider is not polled")
	assert.Zero(t, adder.interval[0])

	_, streaming := adder.added[0].(providertypes.StreamingConfigProvider)
	assert.True(t, streaming, "autodiscovery reads the stream instead of polling")
}

func TestNewListenerRegistersTheFeatureHandlers(t *testing.T) {
	adder := &fakeAdder{}

	_, err := newListener(configmock.NewFromYAML(t, enabledYAML), logmock.New(t), adder, fakeDiscovery{})
	require.NoError(t, err)

	keys := adder.added[0].(interface{ RegisteredKeys() []string }).RegisteredKeys()
	assert.Equal(t, []string{"discovery", "snmp"}, keys)
}
