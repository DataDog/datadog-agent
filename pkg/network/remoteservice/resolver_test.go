// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteservice

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	clientPID = int32(100)
	serverPID = int32(200)
	otherPID  = int32(300)
)

// newResolver returns a Resolver wired with predictable tag callbacks so
// tests can assert which PID was picked.
func newResolver(listeners map[ListenKey]int32) *Resolver {
	return &Resolver{
		GetServiceContext: func(pid int32) []string {
			switch pid {
			case serverPID:
				return []string{"service:server"}
			case otherPID:
				return []string{"service:other"}
			}
			return nil
		},
		GetProcessTags: func(pid int32) []string {
			switch pid {
			case serverPID:
				return []string{"pid_tag:server"}
			case otherPID:
				return []string{"pid_tag:other"}
			}
			return nil
		},
		Listeners: listeners,
	}
}

func TestResolveExactMatch(t *testing.T) {
	r := newResolver(map[ListenKey]int32{
		{IP: "127.0.0.1", Port: 8080}: serverPID,
	})

	tags := r.Resolve(clientPID, "127.0.0.1", 8080, 12345)
	assert.ElementsMatch(t, []string{"service:server", "pid_tag:server"}, tags)
}

func TestResolveIPv4WildcardFallback(t *testing.T) {
	r := newResolver(map[ListenKey]int32{
		{IP: "0.0.0.0", Port: 8080}: serverPID,
	})

	// Connection lands on the loopback IP; listener bound to 0.0.0.0 should match.
	tags := r.Resolve(clientPID, "127.0.0.1", 8080, 12345)
	assert.ElementsMatch(t, []string{"service:server", "pid_tag:server"}, tags)

	// Connection lands on a LAN IP; same wildcard listener should still match.
	tags = r.Resolve(clientPID, "192.168.1.10", 8080, 12345)
	assert.ElementsMatch(t, []string{"service:server", "pid_tag:server"}, tags)
}

func TestResolveIPv6WildcardFallback(t *testing.T) {
	r := newResolver(map[ListenKey]int32{
		{IP: "::", Port: 8080}: serverPID,
	})

	tags := r.Resolve(clientPID, "::1", 8080, 12345)
	assert.ElementsMatch(t, []string{"service:server", "pid_tag:server"}, tags)
}

func TestResolveIPv6WildcardCoversIPv4(t *testing.T) {
	// A dual-stack :: listener (IPV6_V6ONLY=0) accepts IPv4 connections too.
	r := newResolver(map[ListenKey]int32{
		{IP: "::", Port: 8080}: serverPID,
	})

	tags := r.Resolve(clientPID, "127.0.0.1", 8080, 12345)
	assert.ElementsMatch(t, []string{"service:server", "pid_tag:server"}, tags)
}

func TestResolveExactMatchPreferredOverWildcard(t *testing.T) {
	// Two distinct PIDs listening on the same port via different interfaces.
	// A connection to 127.0.0.1 must resolve to the loopback PID, not the
	// wildcard PID.
	r := newResolver(map[ListenKey]int32{
		{IP: "127.0.0.1", Port: 8080}: serverPID,
		{IP: "0.0.0.0", Port: 8080}:   otherPID,
	})

	tags := r.Resolve(clientPID, "127.0.0.1", 8080, 12345)
	assert.ElementsMatch(t, []string{"service:server", "pid_tag:server"}, tags)
}

func TestResolveInterfaceConflictPicksRightPID(t *testing.T) {
	// Same port, two different specific interfaces — the resolver must pick
	// the one matching the connection's destination IP.
	r := newResolver(map[ListenKey]int32{
		{IP: "127.0.0.1", Port: 8080}:    serverPID,
		{IP: "192.168.1.10", Port: 8080}: otherPID,
	})

	tags := r.Resolve(clientPID, "127.0.0.1", 8080, 12345)
	assert.ElementsMatch(t, []string{"service:server", "pid_tag:server"}, tags)

	tags = r.Resolve(clientPID, "192.168.1.10", 8080, 12345)
	assert.ElementsMatch(t, []string{"service:other", "pid_tag:other"}, tags)
}

func TestResolveNoMatch(t *testing.T) {
	r := newResolver(map[ListenKey]int32{
		{IP: "127.0.0.1", Port: 8080}: serverPID,
	})

	assert.Nil(t, r.Resolve(clientPID, "127.0.0.1", 9999, 12345))
}

func TestResolveSelfConnection(t *testing.T) {
	// A connection from a process to its own listening port should not be
	// enriched: the destination tags would be the source's own tags.
	r := newResolver(map[ListenKey]int32{
		{IP: "127.0.0.1", Port: 8080}: clientPID,
	})

	assert.Nil(t, r.Resolve(clientPID, "127.0.0.1", 8080, 12345))
}

func TestResolveIISTagsTakePrecedence(t *testing.T) {
	r := newResolver(map[ListenKey]int32{
		{IP: "127.0.0.1", Port: 8080}: serverPID,
	})
	r.GetIISTags = func(remotePort, localPort int32) []string {
		if remotePort == 8080 && localPort == 12345 {
			return []string{"service:iis-app"}
		}
		return nil
	}

	// IIS tags win over PID-based fallback.
	tags := r.Resolve(clientPID, "127.0.0.1", 8080, 12345)
	assert.Equal(t, []string{"service:iis-app"}, tags)
}

func TestResolveIISTagsFallbackToPID(t *testing.T) {
	// When IIS lookup misses, fall through to PID-based resolution.
	r := newResolver(map[ListenKey]int32{
		{IP: "127.0.0.1", Port: 8080}: serverPID,
	})
	r.GetIISTags = func(_, _ int32) []string { return nil }

	tags := r.Resolve(clientPID, "127.0.0.1", 8080, 12345)
	assert.ElementsMatch(t, []string{"service:server", "pid_tag:server"}, tags)
}
