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
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/nettest"
	"golang.org/x/sys/unix"
	"math/rand/v2"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestAcceptEvent(t *testing.T) {

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_accept_af_inet",
			Expression: `accept.addr.family == AF_INET && process.file.name == "syscall_tester"`,
		},
		{
			ID:         "test_accept_af_inet6",
			Expression: `accept.addr.family == AF_INET6 && process.file.name == "syscall_tester"`,
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

	const MIX = 4000
	const MAX = 5000

	t.Run("accept-af-inet-any-tcp-success", func(t *testing.T) {
		port := rand.IntN(MAX-MIX) + MIX
		go func() {
			err := connectTo("AF_INET", "127.0.0.1", strconv.Itoa(port))
			if err != nil {
				t.Error(err)
			}
		}()
		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "accept", "AF_INET", "0.0.0.0", strconv.Itoa(port))
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "accept", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(unix.AF_INET), event.Accept.AddrFamily, "wrong address family")
			assert.Equal(t, uint16(port), event.Accept.Addr.Port, "wrong address port")
			assert.Equal(t, "127.0.0.1", event.Accept.Addr.IPNet.IP.String(), "wrong address")
			assert.LessOrEqual(t, int64(0), event.Accept.Retval, "wrong retval")
			test.validateConnectSchema(t, event)
		})
	})

	t.Run("accept-af-inet6-any-tcp-success", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		port := rand.IntN(MAX-MIX) + MIX
		go func() {
			err := connectTo("AF_INET6", "::1", strconv.Itoa(port))
			if err != nil {
				t.Error(err)
			}
		}()
		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "accept", "AF_INET6", "::", strconv.Itoa(port))
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "accept", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(unix.AF_INET6), event.Accept.AddrFamily, "wrong address family")
			assert.Equal(t, uint16(port), event.Accept.Addr.Port, "wrong address port")
			assert.Equal(t, "::1", event.Accept.Addr.IPNet.IP.String(), "wrong address")
			assert.LessOrEqual(t, int64(0), event.Accept.Retval, "wrong retval")
			test.validateConnectSchema(t, event)
		})
	})
}

func connectTo(af string, address string, port string) error {
	MAXRetries := 10
	var conn net.Conn

	fmt.Printf("Connecting to: %s %s %s\n", af, address, port)

	if af == "AF_INET6" {
		address = "[" + address + "]"
	} else if af != "AF_INET" {
		return fmt.Errorf("unknown address family %s", af)
	}

	attempts := 0
	for ; attempts < MAXRetries; attempts++ {
		var err error
		conn, err = net.DialTimeout("tcp", address+":"+port, 1000*time.Millisecond)
		time.Sleep(100 * (time.Duration(attempts) + 1) * time.Millisecond)
		if err == nil {
			break
		}
	}
	if attempts == MAXRetries {
		return fmt.Errorf("timed out waiting for connection to %s", address+":"+port)
	}
	if conn != nil {
		_ = conn.Close()
	}
	return nil
}
