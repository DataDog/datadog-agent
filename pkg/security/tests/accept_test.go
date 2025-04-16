// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"context"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/nettest"
	"golang.org/x/sys/unix"
)

func TestAcceptEvent(t *testing.T) {
	SkipIfNotAvailable(t)

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

	t.Run("accept-af-inet-any-tcp-success-no-sockaddrin", func(t *testing.T) {
		if ebpfLessEnabled {
			t.Skip("Not available for ebpfLess")
		}
		port := rand.IntN(MAX-MIX) + MIX

		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "accept", "AF_INET", "0.0.0.0", "127.0.0.1", strconv.Itoa(port), "false")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_accept_af_inet")
			assert.Equal(t, "accept", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(unix.AF_INET), event.Accept.AddrFamily, "wrong address family")
			assert.Equal(t, uint16(port), event.Accept.Addr.Port, "wrong address port")
			assert.Equal(t, "127.0.0.1", event.Accept.Addr.IPNet.IP.String(), "wrong address")
			assert.LessOrEqual(t, int64(0), event.Accept.Retval, "wrong retval")
			test.validateAcceptSchema(t, event)
		})
	})

	t.Run("accept-af-inet-any-tcp-success-sockaddrin", func(t *testing.T) {

		port := rand.IntN(MAX-MIX) + MIX

		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "accept", "AF_INET", "0.0.0.0", "127.0.0.1", strconv.Itoa(port), "true")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_accept_af_inet")
			assert.Equal(t, "accept", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(unix.AF_INET), event.Accept.AddrFamily, "wrong address family")
			assert.Equal(t, "127.0.0.1", event.Accept.Addr.IPNet.IP.String(), "wrong address")
			assert.LessOrEqual(t, int64(0), event.Accept.Retval, "wrong retval")
			test.validateAcceptSchema(t, event)
		})
	})

	t.Run("accept-af-inet6-any-tcp-success-no-sockaddrin", func(t *testing.T) {
		if ebpfLessEnabled {
			t.Skip("Not available for ebpfLess")
		}

		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		port := rand.IntN(MAX-MIX) + MIX

		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "accept", "AF_INET6", "::", "::1", strconv.Itoa(port), "false")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_accept_af_inet6")
			assert.Equal(t, "accept", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(unix.AF_INET6), event.Accept.AddrFamily, "wrong address family")
			assert.Equal(t, uint16(port), event.Accept.Addr.Port, "wrong address port")
			assert.Equal(t, "::1", event.Accept.Addr.IPNet.IP.String(), "wrong address")
			assert.LessOrEqual(t, int64(0), event.Accept.Retval, "wrong retval")
			test.validateAcceptSchema(t, event)
		})
	})

	t.Run("accept-af-inet6-any-tcp-success-sockaddrin", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		port := rand.IntN(MAX-MIX) + MIX

		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "accept", "AF_INET6", "::", "::1", strconv.Itoa(port), "true")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_accept_af_inet6")
			assert.Equal(t, "accept", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(unix.AF_INET6), event.Accept.AddrFamily, "wrong address family")
			assert.Equal(t, "::1", event.Accept.Addr.IPNet.IP.String(), "wrong address")
			assert.LessOrEqual(t, int64(0), event.Accept.Retval, "wrong retval")
			test.validateAcceptSchema(t, event)
		})
	})
}
