// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !aix && !freebsd && !solaris

package datadogextension

import (
	"context"
	"errors"
	"testing"
	"time"

	upstream "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogextension"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/agentcomponents"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configopaque"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/extensioncapabilities"

	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/extension/datadogextension/internal/remoteconfig"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

type child struct {
	starts, stops, snapshots, ready, notReady, statuses, serializers int
	startErr, errorOnStop                                            error
}

func (c *child) Start(context.Context, component.Host) error { c.starts++; return c.startErr }
func (c *child) Shutdown(context.Context) error              { c.stops++; return c.errorOnStop }
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

func testFactory(t *testing.T, metadata, rc *child) (*factory, *Config) {
	t.Helper()
	base := upstream.NewFactory()
	f := &factory{base: extension.NewFactory(base.Type(), base.CreateDefaultConfig,
		func(_ context.Context, _ extension.Settings, c component.Config) (extension.Extension, error) {
			require.Equal(t, "collector-secret", string(c.(*upstream.Config).API.Key))
			return metadata, nil
		}, base.Stability()), newRC: func(_ extension.Settings, c *remoteconfig.Config, r model.Reader) (extension.Extension, error) {
		require.Equal(t, "collector-secret", string(c.API.Key))
		require.Equal(t, "datadoghq.eu", c.API.Site)
		require.Equal(t, "collector-secret", r.GetString("api_key"))
		require.Equal(t, "datadoghq.eu", r.GetString("site"))
		return rc, nil
	}}
	c := f.defaultConfig().(*Config)
	c.Datadog.API.Key = "collector-secret"
	c.Datadog.API.Site = "datadoghq.eu"
	c.RemoteConfig.CacheDirectory = t.TempDir()
	return f, c
}

func TestOptInAndSharedCredentials(t *testing.T) {
	t.Setenv("DD_API_KEY", "unrelated-agent-key")
	t.Setenv("DD_SITE", "datadoghq.com")
	metadata, rc := &child{}, &child{}
	f, c := testFactory(t, metadata, rc)
	set := extension.Settings{ID: component.NewID(component.MustNewType("datadog")), TelemetrySettings: componenttest.NewNopTelemetrySettings()}
	e, err := f.create(t.Context(), set, c)
	require.NoError(t, err)
	require.Same(t, metadata, e)
	require.Equal(t, 0, rc.starts)
	c.RemoteConfig.Enabled = true
	e, err = f.create(t.Context(), set, c)
	require.NoError(t, err)
	require.NoError(t, e.Start(t.Context(), componenttest.NewNopHost()))
	require.Equal(t, 1, metadata.starts)
	require.Equal(t, 1, rc.starts)
	require.NoError(t, e.Shutdown(t.Context()))
	require.NoError(t, e.Shutdown(t.Context()))
	require.Equal(t, 1, rc.stops)
	require.Equal(t, 1, metadata.stops)
}

func TestLifecycleRollback(t *testing.T) {
	metadata, rc := &child{}, &child{startErr: errors.New("listener failed")}
	e := &datadogExtension{metadata: metadata, rc: rc}
	require.ErrorContains(t, e.Start(t.Context(), componenttest.NewNopHost()), "listener failed")
	require.Equal(t, 1, metadata.stops)
	require.NoError(t, e.Shutdown(t.Context()))
	require.Equal(t, 1, metadata.stops)
}

func TestMetadataCapabilitiesPreserved(t *testing.T) {
	metadata := &child{}
	e := &datadogExtension{metadata: metadata, rc: &child{}}
	require.NoError(t, e.NotifyConfigSnapshot(t.Context(), extensioncapabilities.NewConfigSnapshot(confmap.New(), confmap.New())))
	require.NoError(t, e.Ready())
	require.NoError(t, e.NotReady())
	e.ComponentStatusChanged(nil, nil)
	require.Nil(t, e.GetSerializer())
	require.Equal(t, []int{1, 1, 1, 1, 1}, []int{metadata.snapshots, metadata.ready, metadata.notReady, metadata.statuses, metadata.serializers})
}

func TestCollectorDecodingAndListenerIsolation(t *testing.T) {
	f := NewFactory(upstream.NewFactory())
	c := f.CreateDefaultConfig().(*Config)
	conf := confmap.NewFromStringMap(map[string]any{
		"api":           map[string]any{"key": "collector-secret", "site": "datadoghq.eu"},
		"http":          map[string]any{"endpoint": "127.0.0.1:9877"},
		"remote_config": map[string]any{"enabled": true, "endpoint": "127.0.0.1:9234", "cache_directory": t.TempDir(), "refresh_interval": "5s"},
	})
	require.NoError(t, conf.Unmarshal(c))
	require.NoError(t, confmap.Validate(c))
	require.Equal(t, "127.0.0.1:9877", c.Datadog.HTTPConfig.ServerConfig.NetAddr.Endpoint)
	require.Equal(t, "127.0.0.1:9234", c.RemoteConfig.ServerConfig.NetAddr.Endpoint)
	require.Equal(t, 5*time.Second, c.RemoteConfig.RefreshInterval)
	require.Empty(t, c.RemoteConfig.API.Key)
	require.Equal(t, "collector-secret", string(c.rcConfig().API.Key))
	require.Equal(t, component.MustNewType("datadog"), f.Type())
}

func TestRejectSecondCredentialSource(t *testing.T) {
	c := NewFactory(upstream.NewFactory()).CreateDefaultConfig()
	err := confmap.NewFromStringMap(map[string]any{"remote_config": map[string]any{"api": map[string]any{"key": "sdk-secret"}}}).Unmarshal(c)
	require.ErrorContains(t, err, "remote_config.api is not supported")
}

func TestBackendTransportConfiguration(t *testing.T) {
	_, c := testFactory(t, &child{}, &child{})
	c.Datadog.ClientConfig.TLS.CAFile = "/custom-ca.crt"
	c.Datadog.ClientConfig.ProxyURL = "http://corporate-proxy:8080"
	c.RemoteConfig.Enabled = true
	require.NoError(t, confmap.Validate(c))
	require.Equal(t, "/custom-ca.crt", c.rcConfig().Backend.TLS.CAFile)
	require.Equal(t, "http://corporate-proxy:8080", c.rcConfig().Backend.ProxyURL)
	c.Datadog.ClientConfig.TLS.InsecureSkipVerify = true
	require.ErrorContains(t, c.Validate(), "verified backend TLS")
}

func TestCollectorRecursiveValidationWithRCDisabled(t *testing.T) {
	_, c := testFactory(t, &child{}, &child{})
	c.RemoteConfig.CacheDirectory = ""
	require.NoError(t, confmap.Validate(c))
}

func TestBackendCredentialsCannotBeOverridden(t *testing.T) {
	_, c := testFactory(t, &child{}, &child{})
	c.Datadog.ClientConfig.Headers = configopaque.MapList{{Name: "DD-API-KEY", Value: "another-key"}}
	require.NoError(t, c.Validate())
	c.RemoteConfig.Enabled = true
	require.ErrorContains(t, c.Validate(), "api.key for backend authentication")
}
