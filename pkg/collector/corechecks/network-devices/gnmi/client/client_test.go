// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client_test

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/client"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/internal/fakeserver"
)

func testProfile() config.ProfileDefinition {
	return config.ProfileDefinition{
		Name: "test",
		Metrics: []config.MetricConfig{
			{
				Path:   "/openconfig/interfaces/interface/state/counters/in-octets",
				Metric: "snmp.ifHCInOctets",
				Type:   config.MetricTypeMonotonicCount,
				Tags: map[string]string{
					"interface": "name",
				},
			},
			{
				Path:   "/openconfig/interfaces/interface/state/counters/out-octets",
				Metric: "snmp.ifHCOutOctets",
				Type:   config.MetricTypeMonotonicCount,
				Tags: map[string]string{
					"interface": "name",
				},
			},
		},
	}
}

func startServer(t *testing.T) *fakeserver.Server {
	server, err := fakeserver.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})
	return server
}

func newTestClient(t *testing.T, server *fakeserver.Server, opts ...client.Option) *client.Client {
	t.Helper()

	host, port, err := hostPort(server.Addr())
	require.NoError(t, err)

	cfg := client.Config{
		Address:  host,
		Port:     port,
		Username: "user",
		Password: "pass",
		Profile:  testProfile(),
	}
	opts = append([]client.Option{client.WithReconnectDelays(50*time.Millisecond, 200*time.Millisecond)}, opts...)

	c, err := client.New(cfg, opts...)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, c.Close())
	})
	return c
}

func startClient(t *testing.T, c *client.Client) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, c.Start(ctx))
	t.Cleanup(cancel)
	return ctx
}

func waitSubscribeEvent(t *testing.T, server *fakeserver.Server) fakeserver.SubscribeEvent {
	t.Helper()
	select {
	case event := <-server.SubscribeRequests():
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for subscribe event")
		return fakeserver.SubscribeEvent{}
	}
}

func TestSubscribeSyncUpdateAndCacheRead(t *testing.T) {
	server := startServer(t)
	server.SetExpectedCredentials("user", "pass")

	c := newTestClient(t, server)
	startClient(t, c)

	event := waitSubscribeEvent(t, server)
	require.Equal(t, "user", event.Username)
	require.Equal(t, "pass", event.Password)
	require.NotNil(t, event.Request.GetSubscribe())
	require.Equal(t, gnmipb.Encoding_JSON_IETF, event.Request.GetSubscribe().GetEncoding())
	require.Len(t, event.Request.GetSubscribe().GetSubscription(), 2+len(client.MetadataSubscriptionPaths(config.DefaultOpenConfigMetadata())))

	update := fakeserver.InterfaceInOctetsUpdate("eth0", 42)
	require.NoError(t, server.SendUpdate(event.StreamID, update))

	require.Eventually(t, func() bool {
		return c.Synchronized()
	}, 2*time.Second, 10*time.Millisecond)

	require.Eventually(t, func() bool {
		entry, ok := c.Get("/openconfig/interfaces/interface/state/counters/in-octets", map[string]string{"name": "eth0"})
		if !ok {
			return false
		}
		return entry.Value == uint64(42) && entry.Keys["name"] == "eth0"
	}, 2*time.Second, 10*time.Millisecond)
}

func TestReconnectAfterStreamClose(t *testing.T) {
	server := startServer(t)
	server.SetExpectedCredentials("user", "pass")

	c := newTestClient(t, server)
	startClient(t, c)

	first := waitSubscribeEvent(t, server)
	require.NoError(t, server.SendUpdate(first.StreamID, fakeserver.InterfaceInOctetsUpdate("eth0", 1)))

	require.Eventually(t, func() bool {
		_, ok := c.Get("/openconfig/interfaces/interface/state/counters/in-octets", map[string]string{"name": "eth0"})
		return ok
	}, 2*time.Second, 10*time.Millisecond)

	require.NoError(t, server.CloseStream(first.StreamID))

	second := waitSubscribeEvent(t, server)
	require.NotEqual(t, first.StreamID, second.StreamID)

	require.NoError(t, server.SendUpdate(second.StreamID, fakeserver.InterfaceInOctetsUpdate("eth0", 99)))

	require.Eventually(t, func() bool {
		entry, ok := c.Get("/openconfig/interfaces/interface/state/counters/in-octets", map[string]string{"name": "eth0"})
		return ok && entry.Value == uint64(99)
	}, 2*time.Second, 10*time.Millisecond)
}

func TestCancelCleansUp(t *testing.T) {
	server := startServer(t)
	server.SetExpectedCredentials("user", "pass")

	host, port, err := hostPort(server.Addr())
	require.NoError(t, err)

	c, err := client.New(client.Config{
		Address:  host,
		Port:     port,
		Username: "user",
		Password: "pass",
		Profile:  testProfile(),
	}, client.WithReconnectDelays(50*time.Millisecond, 200*time.Millisecond))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, c.Start(ctx))
	waitSubscribeEvent(t, server)

	require.Eventually(t, func() bool {
		return server.ConnectionCount() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	cancel()
	require.NoError(t, c.Close())

	require.Eventually(t, func() bool {
		return server.ActiveStreamCount() == 0
	}, 2*time.Second, 10*time.Millisecond)
}

func TestDeleteRemovesCacheEntry(t *testing.T) {
	server := startServer(t)
	c := newTestClient(t, server)
	startClient(t, c)

	event := waitSubscribeEvent(t, server)
	require.NoError(t, server.SendUpdate(event.StreamID, fakeserver.InterfaceInOctetsUpdate("eth1", 100)))

	require.Eventually(t, func() bool {
		_, ok := c.Get("/openconfig/interfaces/interface/state/counters/in-octets", map[string]string{"name": "eth1"})
		return ok
	}, 2*time.Second, 10*time.Millisecond)

	deletePath := &gnmipb.Path{
		Elem: []*gnmipb.PathElem{
			{Name: "openconfig"},
			{Name: "interfaces"},
			{Name: "interface", Key: map[string]string{"name": "eth1"}},
			{Name: "state"},
			{Name: "counters"},
			{Name: "in-octets"},
		},
	}
	require.NoError(t, server.SendDelete(event.StreamID, deletePath))

	require.Eventually(t, func() bool {
		_, ok := c.Get("/openconfig/interfaces/interface/state/counters/in-octets", map[string]string{"name": "eth1"})
		return !ok
	}, 2*time.Second, 10*time.Millisecond)
}

func TestReplaceUpdatesCache(t *testing.T) {
	server := startServer(t)
	c := newTestClient(t, server)
	startClient(t, c)

	event := waitSubscribeEvent(t, server)
	replace := &gnmipb.Notification{
		Timestamp: time.Now().UnixNano(),
		Update: []*gnmipb.Update{
			fakeserver.InterfaceInOctetsUpdate("eth2", 10),
			fakeserver.InterfaceOutOctetsUpdate("eth2", 20),
		},
	}
	require.NoError(t, server.SendReplace(event.StreamID, replace))

	require.Eventually(t, func() bool {
		inEntry, inOK := c.Get("/openconfig/interfaces/interface/state/counters/in-octets", map[string]string{"name": "eth2"})
		outEntry, outOK := c.Get("/openconfig/interfaces/interface/state/counters/out-octets", map[string]string{"name": "eth2"})
		return inOK && outOK && inEntry.Value == uint64(10) && outEntry.Value == uint64(20)
	}, 2*time.Second, 10*time.Millisecond)
}

func TestNoTightReconnectLoopOnFailure(t *testing.T) {
	server := startServer(t)
	server.SetExpectedCredentials("user", "pass")
	server.SetRejectCredentials(true)

	c := newTestClient(t, server, client.WithReconnectDelays(100*time.Millisecond, 500*time.Millisecond))
	startClient(t, c)

	var (
		lastAttempt int
		attempts    []time.Time
	)
	require.Eventually(t, func() bool {
		current := c.ReconnectAttempts()
		if current > lastAttempt {
			attempts = append(attempts, time.Now())
			lastAttempt = current
		}
		return len(attempts) >= 3
	}, 5*time.Second, 10*time.Millisecond)

	require.GreaterOrEqual(t, attempts[1].Sub(attempts[0]), 50*time.Millisecond)
	require.GreaterOrEqual(t, attempts[2].Sub(attempts[1]), 50*time.Millisecond)

	require.Eventually(t, func() bool {
		status := c.ConnectionStatus()
		return status.LastError != "" && !status.LastErrorAt.IsZero() && !status.NextReconnectAt.IsZero()
	}, 2*time.Second, 10*time.Millisecond)
}

func hostPort(addr string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}
