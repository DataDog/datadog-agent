// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !aix && !freebsd && !solaris

package datadogextension

import (
	"context"
	"errors"
	upstream "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogextension"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/agentcomponents"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/extensioncapabilities"
	"testing"
)

type child struct {
	starts, stops, snapshots, ready, notReady, statuses, serializers int
	startErr                                                         error
}

func (c *child) Start(context.Context, component.Host) error { c.starts++; return c.startErr }
func (c *child) Shutdown(context.Context) error              { c.stops++; return nil }
func (c *child) NotifyConfigSnapshot(context.Context, extensioncapabilities.ConfigSnapshot) error {
	c.snapshots++
	return nil
}
func (c *child) Ready() error    { c.ready++; return nil }
func (c *child) NotReady() error { c.notReady++; return nil }
func (c *child) ComponentStatusChanged(*componentstatus.InstanceID, *componentstatus.Event) {
	c.statuses++
}
func (c *child) GetSerializer() agentcomponents.SerializerWithForwarder { c.serializers++; return nil }

func TestDefaultAndEnabledConfiguration(t *testing.T) {
	f := NewFactory(upstream.NewFactory())
	c := f.CreateDefaultConfig().(*Config)
	c.Datadog.API.Key = "collector-secret"
	require.False(t, c.RemoteConfig.Enabled)
	require.NoError(t, confmap.Validate(c))
	require.NoError(t, confmap.NewFromStringMap(map[string]any{
		"api":           map[string]any{"key": "collector-secret", "site": "datad0g.com"},
		"http":          map[string]any{"endpoint": "127.0.0.1:9877"},
		"remote_config": map[string]any{"enabled": true, "endpoint": "127.0.0.1:18234"},
	}).Unmarshal(c))
	require.NoError(t, confmap.Validate(c))
	require.Equal(t, "127.0.0.1:9877", c.Datadog.HTTPConfig.ServerConfig.NetAddr.Endpoint)
	require.Equal(t, "127.0.0.1:18234", c.RemoteConfig.ServerConfig.NetAddr.Endpoint)
}

func TestRejectSecondCredentialSource(t *testing.T) {
	c := NewFactory(upstream.NewFactory()).CreateDefaultConfig()
	require.ErrorContains(t, confmap.NewFromStringMap(map[string]any{"remote_config": map[string]any{"api": map[string]any{"key": "sdk-secret"}}}).Unmarshal(c), "top-level api.key")
}

func TestDisabledReturnsOriginalMetadata(t *testing.T) {
	metadata := &child{}
	upstreamFactory := upstream.NewFactory()
	base := extension.NewFactory(upstreamFactory.Type(), upstreamFactory.CreateDefaultConfig, func(context.Context, extension.Settings, component.Config) (extension.Extension, error) {
		return metadata, nil
	}, upstreamFactory.Stability())
	f := NewFactory(base)
	c := f.CreateDefaultConfig().(*Config)
	c.Datadog.API.Key = "collector-secret"
	e, err := f.Create(t.Context(), extension.Settings{ID: component.NewID(f.Type()), TelemetrySettings: componenttest.NewNopTelemetrySettings()}, c)
	require.NoError(t, err)
	require.Same(t, metadata, e)
	factories := map[component.Type]extension.Factory{base.Type(): base}
	WrapFactory(factories)
	wrapped := factories[base.Type()]
	WrapFactory(factories)
	require.Same(t, wrapped, factories[base.Type()])
}

func TestLifecycleRollbackAndMetadataDelegation(t *testing.T) {
	metadata, rc := &child{}, &child{startErr: errors.New("listener failed")}
	e := &datadogExtension{metadata: metadata, rc: rc}
	require.ErrorContains(t, e.Start(t.Context(), componenttest.NewNopHost()), "listener failed")
	require.Equal(t, 1, metadata.stops)
	require.NoError(t, e.Shutdown(t.Context()))
	require.Equal(t, 1, metadata.stops)
	require.NoError(t, e.NotifyConfigSnapshot(t.Context(), extensioncapabilities.NewConfigSnapshot(confmap.New(), confmap.New())))
	require.NoError(t, e.Ready())
	require.NoError(t, e.NotReady())
	e.ComponentStatusChanged(nil, nil)
	require.Nil(t, e.GetSerializer())
	require.Equal(t, []int{1, 1, 1, 1, 1}, []int{metadata.snapshots, metadata.ready, metadata.notReady, metadata.statuses, metadata.serializers})
}

func TestRejectAPIKeyInAdditionalHeaders(t *testing.T) {
	c := NewFactory(upstream.NewFactory()).CreateDefaultConfig().(*Config)
	require.NoError(t, confmap.NewFromStringMap(map[string]any{
		"api":           map[string]any{"key": "collector-secret"},
		"headers":       map[string]any{"DD-API-KEY": "second-secret"},
		"remote_config": map[string]any{"enabled": true},
	}).Unmarshal(c))
	require.ErrorContains(t, c.Validate(), "not HTTP headers")
}
