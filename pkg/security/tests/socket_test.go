// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestSocketEvent(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_socket_af_inet_tcp",
			Expression: `socket.domain == AF_INET && socket.type == SOCK_STREAM && socket.protocol == IPPROTO_TCP && process.file.name == "testsuite"`,
		},
		{
			ID:         "test_socket_af_inet_udp",
			Expression: `socket.domain == AF_INET && socket.type == SOCK_DGRAM && socket.protocol == IPPROTO_UDP && process.file.name == "testsuite"`,
		},
		{
			ID:         "test_socket_af_unix_stream",
			Expression: `socket.domain == AF_UNIX && socket.type == SOCK_STREAM && process.file.name == "testsuite"`,
		},
		{
			ID:         "test_socket_af_inet_raw_icmp",
			Expression: `socket.domain == AF_INET && socket.type == SOCK_RAW && socket.protocol == IPPROTO_ICMP && process.file.name == "testsuite"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("socket-af-inet-tcp", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
			if err != nil {
				return err
			}
			syscall.Close(fd)
			return nil
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "socket", event.GetType(), "wrong event type")
			t.Logf("Socket event: domain=%d (expected %d), type=%d (expected %d), protocol=%d (expected %d), retval=%d",
				event.Socket.Domain, syscall.AF_INET,
				event.Socket.Type, syscall.SOCK_STREAM,
				event.Socket.Protocol, syscall.IPPROTO_TCP,
				event.Socket.Retval)

			assert.True(t, event.Socket.Domain == syscall.AF_INET, "socket domain should be AF_INET")
			assert.True(t, event.Socket.Type&syscall.SOCK_STREAM == syscall.SOCK_STREAM, "socket type should be SOCK_STREAM")
			assert.True(t, event.Socket.Protocol == syscall.IPPROTO_TCP, "socket protocol should be IPPROTO_TCP")
			assert.True(t, event.Socket.Retval > 0, "socket retval should be valid")
		})
	})

	t.Run("socket-af-inet-udp", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, syscall.IPPROTO_UDP)
			if err != nil {
				return err
			}
			syscall.Close(fd)
			return nil
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "socket", event.GetType(), "wrong event type")
			t.Logf("Socket event: domain=%d (expected %d), type=%d (expected %d), protocol=%d (expected %d), retval=%d",
				event.Socket.Domain, syscall.AF_INET,
				event.Socket.Type, syscall.SOCK_DGRAM,
				event.Socket.Protocol, syscall.IPPROTO_UDP,
				event.Socket.Retval)

			assert.True(t, event.Socket.Domain == syscall.AF_INET, "socket domain should be AF_INET")
			assert.True(t, event.Socket.Type&syscall.SOCK_DGRAM == syscall.SOCK_DGRAM, "socket type should be SOCK_DGRAM")
			assert.True(t, event.Socket.Protocol == syscall.IPPROTO_UDP, "socket protocol should be IPPROTO_UDP")
			assert.True(t, event.Socket.Retval > 0, "socket retval should be valid")
		})
	})

	t.Run("socket-af-unix-stream", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
			if err != nil {
				return err
			}
			syscall.Close(fd)
			return nil
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "socket", event.GetType(), "wrong event type")
			t.Logf("Socket event: domain=%d (expected %d), type=%d (expected %d), protocol=%d (expected %d), retval=%d",
				event.Socket.Domain, syscall.AF_UNIX,
				event.Socket.Type, syscall.SOCK_STREAM,
				event.Socket.Protocol, 0,
				event.Socket.Retval)

			assert.True(t, event.Socket.Domain == syscall.AF_UNIX, "socket domain should be AF_UNIX")
			assert.True(t, event.Socket.Type&syscall.SOCK_STREAM == syscall.SOCK_STREAM, "socket type should be SOCK_STREAM")
			assert.True(t, event.Socket.Protocol == 0, "socket protocol should be 0 (default for AF_UNIX)")
			assert.True(t, event.Socket.Retval > 0, "socket retval should be valid")
		})
	})

	t.Run("socket-af-inet-raw-icmp", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_ICMP)
			if err != nil {
				// Raw sockets typically require root privileges
				// If we get EPERM (permission denied), that's expected in some environments
				if err == syscall.EPERM {
					t.Skipf("Skipping raw socket test: permission denied (need root privileges)")
					return nil
				}
				return err
			}
			syscall.Close(fd)
			return nil
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "socket", event.GetType(), "wrong event type")
			t.Logf("Socket event: domain=%d (expected %d), type=%d (expected %d), protocol=%d (expected %d), retval=%d",
				event.Socket.Domain, syscall.AF_INET,
				event.Socket.Type, syscall.SOCK_RAW,
				event.Socket.Protocol, syscall.IPPROTO_ICMP,
				event.Socket.Retval)

			assert.True(t, event.Socket.Domain == syscall.AF_INET, "socket domain should be AF_INET")
			assert.True(t, event.Socket.Type&syscall.SOCK_RAW == syscall.SOCK_RAW, "socket type should be SOCK_RAW")
			assert.True(t, event.Socket.Protocol == syscall.IPPROTO_ICMP, "socket protocol should be IPPROTO_ICMP")
			assert.True(t, event.Socket.Retval > 0, "socket retval should be valid")
		})
	})
}
