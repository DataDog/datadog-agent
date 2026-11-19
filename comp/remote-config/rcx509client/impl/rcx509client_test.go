// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcx509clientimpl

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	cfgcomp "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	rcx509client "github.com/DataDog/datadog-agent/comp/remote-config/rcx509client/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/version"
	remoteconfigv1 "github.com/DataDog/libdd-rc/ffi-hosts/go/rcproto/magic_tunnel/remote_config"
)

type fakeClient struct {
	started   chan struct{}
	stop      chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	closeErr  error

	// closeBlock, if set, makes Close wait until it is closed.
	closeBlock chan struct{}

	// hostTags records each SetHostTags call, and whether Start had been
	// called by then.
	hostTagsMu        sync.Mutex
	hostTags          [][]string
	hostTagsAfterStop bool
	startCalled       bool
	hostTagsBeforeRun []bool
	hostTagsCalled    chan struct{}
}

func (c *fakeClient) SetHostTags(tags []string) error {
	c.hostTagsMu.Lock()
	defer c.hostTagsMu.Unlock()
	select {
	case <-c.stop:
		c.hostTagsAfterStop = true
	default:
	}
	c.hostTags = append(c.hostTags, tags)
	c.hostTagsBeforeRun = append(c.hostTagsBeforeRun, !c.startCalled)
	select {
	case c.hostTagsCalled <- struct{}{}:
	default:
	}
	return nil
}

func (c *fakeClient) hostTagsCalls() [][]string {
	c.hostTagsMu.Lock()
	defer c.hostTagsMu.Unlock()
	return slices.Clone(c.hostTags)
}

func newFakeClient() *fakeClient {
	return &fakeClient{
		started:        make(chan struct{}),
		stop:           make(chan struct{}),
		hostTagsCalled: make(chan struct{}, 1),
	}
}

func (c *fakeClient) Start() error {
	c.hostTagsMu.Lock()
	c.startCalled = true
	c.hostTagsMu.Unlock()
	c.startOnce.Do(func() { close(c.started) })
	<-c.stop
	return nil
}

func (c *fakeClient) Close() error {
	if c.closeBlock != nil {
		<-c.closeBlock
	}
	c.stopOnce.Do(func() { close(c.stop) })
	return c.closeErr
}

const testHostname = "test-host"

func newDependencies(t *testing.T, overrides map[string]interface{}, factory rcx509client.ClientFactory) (Dependencies, *compdef.TestLifecycle) {
	t.Helper()
	lifecycle := compdef.NewTestLifecycle(t)
	hostname, _ := hostnamemock.NewMock(testHostname)
	return Dependencies{
		Lifecycle: lifecycle,
		Config:    cfgcomp.NewMockWithOverrides(t, overrides),
		Logger:    logmock.New(t),
		Hostname:  hostname,
		Tagger:    option.None[tagger.Component](),
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

func TestStopReturnsWhenContextDone(t *testing.T) {
	client := newFakeClient()
	client.closeBlock = make(chan struct{})
	deps, lifecycle := newDependencies(t, map[string]interface{}{enabledSetting: true}, func(rcx509client.ClientConfig) (rcx509client.Client, error) {
		return client, nil
	})
	provided := New(deps)
	require.NoError(t, lifecycle.Start(context.Background()))

	// Close blocks until closeBlock is closed, so Stop can only return through ctx.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, lifecycle.Stop(ctx), context.Canceled)
	require.Equal(t, rcx509client.StateStopping, provided.Comp.State())

	// Let the background shutdown finish before the test ends.
	close(client.closeBlock)
	provided.Comp.(*component).wg.Wait()
}

// TestHostTagsSetBeforeStart verifies the host tags, including the hostname,
// are handed to the client before it starts connecting. The other tags come
// from rctags, shared with the existing Remote Config client.
func TestHostTagsSetBeforeStart(t *testing.T) {
	client := newFakeClient()
	deps, lifecycle := newDependencies(t, map[string]interface{}{enabledSetting: true}, func(rcx509client.ClientConfig) (rcx509client.Client, error) {
		return client, nil
	})
	New(deps)

	require.NoError(t, lifecycle.Start(context.Background()))
	<-client.started
	require.NoError(t, lifecycle.Stop(context.Background()))

	calls := client.hostTagsCalls()
	require.NotEmpty(t, calls)
	require.True(t, client.hostTagsBeforeRun[0], "first SetHostTags call happened after Start")
	require.Contains(t, calls[0], "host:"+testHostname)
}

// TestHostTagsRefreshStopsBeforeClose verifies the tags are refreshed while
// running, and that no refresh reaches the client once it is closed.
func TestHostTagsRefreshStopsBeforeClose(t *testing.T) {
	client := newFakeClient()
	deps, lifecycle := newDependencies(t, map[string]interface{}{enabledSetting: true}, func(rcx509client.ClientConfig) (rcx509client.Client, error) {
		return client, nil
	})
	provided := New(deps)
	provided.Comp.(*component).refreshInterval = time.Millisecond

	require.NoError(t, lifecycle.Start(context.Background()))
	for range 3 { // the initial set plus at least two refreshes
		select {
		case <-client.hostTagsCalled:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for host tags to be refreshed")
		}
	}
	require.NoError(t, lifecycle.Stop(context.Background()))

	client.hostTagsMu.Lock()
	defer client.hostTagsMu.Unlock()
	require.False(t, client.hostTagsAfterStop, "SetHostTags was called after Close")
}
