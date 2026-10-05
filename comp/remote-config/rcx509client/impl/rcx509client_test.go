// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcx509clientimpl

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	cfgcomp "github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	rcx509client "github.com/DataDog/datadog-agent/comp/remote-config/rcx509client/def"
	"github.com/DataDog/datadog-agent/pkg/version"
	remoteconfigv1 "github.com/DataDog/libdd-rc/ffi-hosts/go/rcproto/magic_tunnel/remote_config"
)

type fakeClient struct {
	started   chan struct{}
	stop      chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	closeErr  error
}

func newFakeClient() *fakeClient {
	return &fakeClient{
		started: make(chan struct{}),
		stop:    make(chan struct{}),
	}
}

func (c *fakeClient) Start() error {
	c.startOnce.Do(func() { close(c.started) })
	<-c.stop
	return nil
}

func (c *fakeClient) Close() error {
	c.stopOnce.Do(func() { close(c.stop) })
	return c.closeErr
}

func newDependencies(t *testing.T, overrides map[string]interface{}, factory rcx509client.ClientFactory) (Dependencies, *compdef.TestLifecycle) {
	t.Helper()
	lifecycle := compdef.NewTestLifecycle(t)
	return Dependencies{
		Lifecycle: lifecycle,
		Config:    cfgcomp.NewMockWithOverrides(t, overrides),
		Logger:    logmock.New(t),
		Factory:   factory,
	}, lifecycle
}

func TestDisabledByDefault(t *testing.T) {
	deps, lifecycle := newDependencies(t, nil, nil)
	provided := New(deps)

	require.Equal(t, rcx509client.StateDisabled, provided.Comp.State())
	lifecycle.AssertHooksNumber(0)
}

func TestDisabledWhenRemoteConfigIsDisabled(t *testing.T) {
	called := false
	deps, lifecycle := newDependencies(t, map[string]interface{}{
		enabledSetting:                 true,
		"remote_configuration.enabled": false,
	}, func(rcx509client.ClientConfig) (rcx509client.Client, error) {
		called = true
		return newFakeClient(), nil
	})

	provided := New(deps)
	require.Equal(t, rcx509client.StateDisabled, provided.Comp.State())
	require.False(t, called)
	lifecycle.AssertHooksNumber(0)
}

func TestEnabledWithoutFactoryFailsClosed(t *testing.T) {
	deps, lifecycle := newDependencies(t, map[string]interface{}{enabledSetting: true}, nil)
	provided := New(deps)

	require.Equal(t, rcx509client.StateFailed, provided.Comp.State())
	lifecycle.AssertHooksNumber(0)
}

func TestClientConfigurationAndLifecycle(t *testing.T) {
	client := newFakeClient()
	var gotConfig rcx509client.ClientConfig
	deps, lifecycle := newDependencies(t, map[string]interface{}{
		enabledSetting:                           true,
		"api_key":                                "default-key",
		"remote_configuration.api_key":           "  override-key\n",
		debugPingEnabledSetting:                  true,
		"remote_configuration.rc_dd_url":         "https://config.example.test/base/path",
		"remote_configuration.enabled":           true,
		"remote_configuration.no_tls":            false,
		"remote_configuration.no_tls_validation": false,
	}, func(config rcx509client.ClientConfig) (rcx509client.Client, error) {
		gotConfig = config
		return client, nil
	})

	provided := New(deps)
	require.Equal(t, rcx509client.StateInitialized, provided.Comp.State())
	lifecycle.AssertHooksNumber(1)
	require.Equal(t, "wss://config.example.test/base/path/api/v2/ws", gotConfig.URL)
	require.Equal(t, appName, gotConfig.AppName)
	require.Equal(t, version.AgentVersion, gotConfig.Version)
	require.Equal(t, "override-key", gotConfig.APIKey)
	require.NotNil(t, gotConfig.HTTPClient)
	require.IsType(t, &http.Transport{}, gotConfig.HTTPClient.Transport)
	require.True(t, gotConfig.DebugPingEnabled)

	require.NoError(t, lifecycle.Start(context.Background()))
	require.Equal(t, rcx509client.StateRunning, provided.Comp.State())
	require.NoError(t, lifecycle.Stop(context.Background()))
	require.Equal(t, rcx509client.StateStopped, provided.Comp.State())
	select {
	case <-client.started:
	default:
		t.Fatal("client Start was not called")
	}
}

func TestDebugPingHandler(t *testing.T) {
	payload, err := proto.Marshal(&remoteconfigv1.PingRequest{
		ConnectionId: "staging-connection",
		Reason:       "must not be logged",
		Payload:      []byte("must not be logged"),
	})
	require.NoError(t, err)

	responsePayload, err := newDebugPingHandler(logmock.New(t))(context.Background(), 42, payload)
	require.NoError(t, err)

	var response remoteconfigv1.PingResponse
	require.NoError(t, proto.Unmarshal(responsePayload, &response))
	require.NotNil(t, response.Now)
	require.NoError(t, response.Now.CheckValid())
}

func TestDebugPingHandlerRejectsMalformedPayload(t *testing.T) {
	_, err := newDebugPingHandler(logmock.New(t))(context.Background(), 42, []byte("not protobuf"))
	require.ErrorContains(t, err, "decode x509 debug ping request")
}

func TestPlaintextRequiresExistingRemoteConfigOptIn(t *testing.T) {
	called := false
	deps, lifecycle := newDependencies(t, map[string]interface{}{
		enabledSetting:                   true,
		"remote_configuration.rc_dd_url": "http://config.example.test/base",
	}, func(rcx509client.ClientConfig) (rcx509client.Client, error) {
		called = true
		return newFakeClient(), nil
	})

	provided := New(deps)
	require.Equal(t, rcx509client.StateFailed, provided.Comp.State())
	require.False(t, called)
	lifecycle.AssertHooksNumber(0)
}

func TestPlaintextUsesWebSocketWhenExplicitlyAllowed(t *testing.T) {
	client := newFakeClient()
	var gotConfig rcx509client.ClientConfig
	deps, lifecycle := newDependencies(t, map[string]interface{}{
		enabledSetting:                   true,
		"remote_configuration.no_tls":    true,
		"remote_configuration.rc_dd_url": "http://config.example.test/base",
	}, func(config rcx509client.ClientConfig) (rcx509client.Client, error) {
		gotConfig = config
		return client, nil
	})

	provided := New(deps)
	require.Equal(t, rcx509client.StateInitialized, provided.Comp.State())
	require.Equal(t, "ws://config.example.test/base/api/v2/ws", gotConfig.URL)
	lifecycle.AssertHooksNumber(1)
}

func TestFactoryFailureDoesNotRegisterLifecycle(t *testing.T) {
	deps, lifecycle := newDependencies(t, map[string]interface{}{enabledSetting: true}, func(rcx509client.ClientConfig) (rcx509client.Client, error) {
		return nil, errors.New("factory failed")
	})

	provided := New(deps)
	require.Equal(t, rcx509client.StateFailed, provided.Comp.State())
	lifecycle.AssertHooksNumber(0)
}

func TestNilClientDoesNotRegisterLifecycle(t *testing.T) {
	deps, lifecycle := newDependencies(t, map[string]interface{}{enabledSetting: true}, func(rcx509client.ClientConfig) (rcx509client.Client, error) {
		return nil, nil
	})

	provided := New(deps)
	require.Equal(t, rcx509client.StateFailed, provided.Comp.State())
	lifecycle.AssertHooksNumber(0)
}

func TestCloseFailureIsReported(t *testing.T) {
	client := newFakeClient()
	client.closeErr = errors.New("close failed")
	deps, lifecycle := newDependencies(t, map[string]interface{}{enabledSetting: true}, func(rcx509client.ClientConfig) (rcx509client.Client, error) {
		return client, nil
	})
	provided := New(deps)

	require.NoError(t, lifecycle.Start(context.Background()))
	require.ErrorContains(t, lifecycle.Stop(context.Background()), "close failed")
	require.Equal(t, rcx509client.StateFailed, provided.Comp.State())
	select {
	case <-client.started:
	default:
		t.Fatal("client Start was not called")
	}
}
