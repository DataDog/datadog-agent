// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package intakeroutingimpl

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	intakerouting "github.com/DataDog/datadog-agent/comp/intakerouting/def"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

func TestIsPublicAddress(t *testing.T) {
	public := []string{"3.233.146.1", "34.149.66.1", "2600:1f18:24e6:b900::1", "::ffff:3.233.146.1"}
	for _, addr := range public {
		assert.True(t, isPublicAddress(netip.MustParseAddr(addr)), addr)
	}

	nonPublic := []string{
		// RFC 1918
		"10.0.1.5", "172.16.0.1", "172.31.255.254", "192.168.1.1", "::ffff:10.0.1.5",
		// RFC 4193
		"fd12:3456:789a::1",
		// RFC 6598
		"100.64.0.1", "100.127.255.254",
		"127.0.0.1", "::1", "169.254.169.254", "fe80::1", "0.0.0.0", "::", "224.0.0.1",
	}
	for _, addr := range nonPublic {
		assert.False(t, isPublicAddress(netip.MustParseAddr(addr)), addr)
	}
}

func lookupReturning(addrs ...string) func(context.Context, string, string) ([]netip.Addr, error) {
	return func(context.Context, string, string) ([]netip.Addr, error) {
		parsed := make([]netip.Addr, len(addrs))
		for i, addr := range addrs {
			parsed[i] = netip.MustParseAddr(addr)
		}
		return parsed, nil
	}
}

func TestNetPathCheckerAddresses(t *testing.T) {
	tests := []struct {
		name   string
		lookup func(context.Context, string, string) ([]netip.Addr, error)
		reason string
	}{
		{"public", lookupReturning("3.233.146.1", "2600:1f18:24e6:b900::1"), ""},
		{"private", lookupReturning("10.0.1.5"), pathPrivateAddress},
		// A dual-stack PrivateLink endpoint: its IPv6 address is a global one.
		{"dual-stack PrivateLink", lookupReturning("2600:1f18:24e6:b900::1", "10.0.1.5"), pathPrivateAddress},
		{"shared address space", lookupReturning("100.64.3.4"), pathPrivateAddress},
		{"no address", lookupReturning(), pathLookupFailed},
		{"lookup error", func(context.Context, string, string) ([]netip.Addr, error) {
			return nil, errors.New("no such host")
		}, pathLookupFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &netPathChecker{lookup: tt.lookup}
			reason, err := c.check(context.Background(), intakerouting.Endpoint{Host: defaultHost})
			assert.Equal(t, tt.reason, reason)
			if tt.reason == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestNetPathCheckerResolvesAbsoluteName(t *testing.T) {
	var looked []string
	c := &netPathChecker{lookup: func(_ context.Context, network, host string) ([]netip.Addr, error) {
		assert.Equal(t, "ip", network)
		looked = append(looked, host)
		return []netip.Addr{netip.MustParseAddr("3.233.146.1")}, nil
	}}
	for _, host := range []string{defaultHost, defaultHost + "."} {
		reason, err := c.check(context.Background(), intakerouting.Endpoint{Host: host})
		require.NoError(t, err)
		assert.Empty(t, reason)
	}
	assert.Equal(t, []string{defaultHost + ".", defaultHost + "."}, looked)
}

func TestNetPathCheckerProxy(t *testing.T) {
	lookupCalled := false
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		lookupCalled = true
		return []netip.Addr{netip.MustParseAddr("3.233.146.1")}, nil
	}

	proxied := &netPathChecker{lookup: lookup, proxy: func(req *http.Request) (*url.URL, error) {
		assert.Equal(t, "https", req.URL.Scheme)
		assert.Equal(t, defaultHost+":8443", req.URL.Host)
		return url.Parse("http://proxy.internal:3128")
	}}
	reason, err := proxied.check(context.Background(), intakerouting.Endpoint{Host: defaultHost, Port: 8443})
	assert.Equal(t, pathProxied, reason)
	assert.Error(t, err)
	assert.False(t, lookupCalled, "a proxied host is not resolved")

	invalid := &netPathChecker{lookup: lookup, proxy: func(*http.Request) (*url.URL, error) {
		return nil, errors.New("invalid proxy URL")
	}}
	reason, _ = invalid.check(context.Background(), intakerouting.Endpoint{Host: defaultHost})
	assert.Equal(t, pathProxied, reason)

	// No proxy for this host: it is resolved.
	direct := &netPathChecker{lookup: lookup, proxy: func(*http.Request) (*url.URL, error) { return nil, nil }}
	reason, err = direct.check(context.Background(), intakerouting.Endpoint{Host: defaultHost})
	require.NoError(t, err)
	assert.Empty(t, reason)
	assert.True(t, lookupCalled)
}

// TestNetPathCheckerAgentProxySettings checks the path checker against the transport the Agent
// builds from its proxy settings.
func TestNetPathCheckerAgentProxySettings(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"proxy.https":    "http://user:secret@proxy.internal:3128",
		"proxy.no_proxy": []string{"agent-http-intake.logs.datadoghq.eu"},
	})
	c := newNetPathChecker(httputils.CreateHTTPTransport(cfg))
	c.lookup = lookupReturning("3.233.146.1")

	reason, err := c.check(context.Background(), intakerouting.Endpoint{Host: defaultHost})
	assert.Equal(t, pathProxied, reason)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret")

	reason, err = c.check(context.Background(), intakerouting.Endpoint{Host: "agent-http-intake.logs.datadoghq.eu"})
	require.NoError(t, err)
	assert.Empty(t, reason)
}
