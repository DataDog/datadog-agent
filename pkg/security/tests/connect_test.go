// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"

	"golang.org/x/net/nettest"
	"golang.org/x/sys/unix"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestConnectEvent(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_connect_af_inet",
			Expression: `connect.server.addr.family == AF_INET && process.file.name == "syscall_tester"`,
		},
		{
			ID:         "test_connect_af_inet6",
			Expression: `connect.server.addr.family == AF_INET6 && process.file.name == "syscall_tester"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("connect-af-inet-any-tcp-success", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)
		defer wg.Wait()

		done := make(chan struct{})
		defer close(done)
		go func() {
			defer wg.Done()
			err := bindAndAcceptConnection("tcp", ":4242", done)
			if err != nil {
				t.Fatal(err)
			}
		}()

		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "connect", "AF_INET", "any", "tcp")
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "connect", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(unix.AF_INET), event.Connect.AddrFamily, "wrong address family")
			assert.Equal(t, uint16(4242), event.Connect.Addr.Port, "wrong address port")
			assert.Equal(t, string("0.0.0.0/32"), event.Connect.Addr.IPNet.String(), "wrong address")
			assert.Equal(t, int64(0), event.Connect.Retval, "wrong retval")
			test.validateConnectSchema(t, event)
		})
	})

	t.Run("connect-af-inet-any-udp-success", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)
		defer wg.Wait()

		done := make(chan struct{})
		defer close(done)
		go func() {
			defer wg.Done()
			err := bindAndAcceptConnection("udp", ":4242", done)
			if err != nil {
				t.Fatal(err)
			}
		}()

		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "connect", "AF_INET", "any", "udp")
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "connect", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(unix.AF_INET), event.Connect.AddrFamily, "wrong address family")
			assert.Equal(t, uint16(4242), event.Connect.Addr.Port, "wrong address port")
			assert.Equal(t, string("0.0.0.0/32"), event.Connect.Addr.IPNet.String(), "wrong address")
			assert.Equal(t, int64(0), event.Connect.Retval, "wrong retval")
			test.validateConnectSchema(t, event)
		})
	})

	t.Run("connect-af-inet6-any-tcp-success", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		var wg sync.WaitGroup
		wg.Add(1)
		defer wg.Wait()

		done := make(chan struct{})
		defer close(done)
		go func() {
			defer wg.Done()
			err := bindAndAcceptConnection("tcp", ":4242", done)
			if err != nil {
				t.Fatal(err)
			}
		}()

		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "connect", "AF_INET6", "any", "tcp")
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "connect", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(unix.AF_INET6), event.Connect.AddrFamily, "wrong address family")
			assert.Equal(t, uint16(4242), event.Connect.Addr.Port, "wrong address port")
			assert.Equal(t, string("::/128"), event.Connect.Addr.IPNet.String(), "wrong address")
			assert.Equal(t, int64(0), event.Connect.Retval, "wrong retval")
			test.validateConnectSchema(t, event)
		})
	})

	t.Run("connect-af-inet6-any-udp-success", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		var wg sync.WaitGroup
		wg.Add(1)
		defer wg.Wait()

		done := make(chan struct{})
		defer close(done)
		go func() {
			defer wg.Done()
			err := bindAndAcceptConnection("udp", ":4242", done)
			if err != nil {
				t.Fatal(err)
			}
		}()

		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "connect", "AF_INET6", "any", "udp")
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "connect", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(unix.AF_INET6), event.Connect.AddrFamily, "wrong address family")
			assert.Equal(t, uint16(4242), event.Connect.Addr.Port, "wrong address port")
			assert.Equal(t, string("::/128"), event.Connect.Addr.IPNet.String(), "wrong address")
			assert.Equal(t, int64(0), event.Connect.Retval, "wrong retval")
			test.validateConnectSchema(t, event)
		})
	})

}

func bindAndAcceptConnection(proto, address string, done chan struct{}) error {
	switch proto {
	case "tcp":
		listener, err := net.Listen(proto, address)
		if err != nil {
			return fmt.Errorf("failed to bind to %s:%s: %w", proto, address, err)
		}
		defer listener.Close()

		// Start a goroutine to accept connections continuously
		go func() {
			for {
				c, err := listener.Accept()
				if err != nil {
					fmt.Printf("accept error: %v\n", err)
					return
				}
				fmt.Println("Connection accepted")
				defer c.Close()
			}
		}()

	case "udp", "unixgram":
		conn, err := net.ListenPacket(proto, address)
		if err != nil {
			return fmt.Errorf("failed to bind to %s:%s: %w", proto, address, err)
		}
		defer conn.Close()

	default:
		return fmt.Errorf("unsupported protocol: %s", proto)
	}

	// Wait for the test to complete before returning
	<-done
	return nil
}
