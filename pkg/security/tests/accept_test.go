// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"strconv"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/iceber/iouring-go"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/nettest"
	"golang.org/x/sys/unix"
)

func TestAcceptEvent(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_accept_af_inet",
			Expression: `accept.addr.family == AF_INET && process.file.name in [ "syscall_tester", "testsuite" ]`,
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

	const MIN = 4000
	const MAX = 5000

	t.Run("accept-af-inet-any-tcp-success-no-sockaddrin", func(t *testing.T) {
		if ebpfLessEnabled {
			t.Skip("Not available for ebpfLess")
		}
		port := rand.IntN(MAX-MIN) + MIN

		test.WaitSignalFromRule(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "accept", "AF_INET", "0.0.0.0", "127.0.0.1", strconv.Itoa(port), "false")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_accept_af_inet")
			assert.Equal(t, "accept", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(unix.AF_INET), event.Accept.AddrFamily, "wrong address family")
			assert.Equal(t, uint16(port), event.Accept.Addr.Port, "wrong address port")
			assert.Equal(t, "127.0.0.1", event.Accept.Addr.IPNet.IP.String(), "wrong address")
			assert.LessOrEqual(t, int64(0), event.Accept.Retval, "wrong retval")
			test.validateAcceptSchema(t, event)
		}, "test_accept_af_inet")
	})

	t.Run("accept-af-inet-any-tcp-success-sockaddrin", func(t *testing.T) {

		port := rand.IntN(MAX-MIN) + MIN

		test.WaitSignalFromRule(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "accept", "AF_INET", "0.0.0.0", "127.0.0.1", strconv.Itoa(port), "true")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_accept_af_inet")
			assert.Equal(t, "accept", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(unix.AF_INET), event.Accept.AddrFamily, "wrong address family")
			assert.Equal(t, "127.0.0.1", event.Accept.Addr.IPNet.IP.String(), "wrong address")
			assert.LessOrEqual(t, int64(0), event.Accept.Retval, "wrong retval")
			test.validateAcceptSchema(t, event)
		}, "test_accept_af_inet")
	})

	t.Run("accept-af-inet-any-tcp-success-sockaddrin-io-uring", func(t *testing.T) {
		SkipIfNotAvailable(t)

		port := rand.IntN(MAX-MIN) + MIN

		fd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, unix.IPPROTO_TCP)
		if err != nil {
			t.Fatal(err)
		}
		defer unix.Close(fd)

		sa := unix.SockaddrInet4{
			Port: port,
		}
		if err := unix.Bind(fd, &sa); err != nil {
			t.Fatal(err)
		}

		if err := unix.Listen(fd, 10); err != nil {
			t.Fatal(err)
		}

		iour, err := iouring.New(1)
		if err != nil {
			if errors.Is(err, unix.ENOTSUP) {
				t.Fatal(err)
			}
			t.Skip("io_uring not supported")
		}
		defer iour.Close()

		acceptAddr := unix.SockaddrInet4{}

		connectAddr := unix.SockaddrInet4{
			Port: port,
			Addr: [4]byte(net.IPv4(127, 0, 0, 1)),
		}

		prepRequest, err := iouring.Accept(int32(fd), &acceptAddr)
		if err != nil {
			t.Fatal(err)
		}

		client, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, unix.IPPROTO_TCP)
		if err != nil {
			t.Fatal(err)
		}
		defer unix.Close(client)

		ch := make(chan iouring.Result, 1)

		test.WaitSignalFromRule(t, func() error {
			errChan := make(chan error, 1)
			go func() {
				errChan <- unix.Connect(client, &connectAddr)
			}()

			if _, err = iour.SubmitRequest(prepRequest, ch); err != nil {
				return err
			}

			result := <-ch
			ret, err := result.ReturnInt()
			if err != nil {
				if err == syscall.EBADF || err == syscall.EINVAL {
					return ErrSkipTest{fmt.Sprintf("accept not supported by io_uring: %s", err)}
				}
				return fmt.Errorf("io_uring error: %w", err)
			}

			if ret < 0 {
				return fmt.Errorf("failed to accept with io_uring: %d", ret)
			}

			return <-errChan
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_accept_af_inet")
			assert.Equal(t, "accept", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(unix.AF_INET), event.Accept.AddrFamily, "wrong address family")
			assert.Equal(t, "127.0.0.1", event.Accept.Addr.IPNet.IP.String(), "wrong address")
			assert.LessOrEqual(t, int64(0), event.Accept.Retval, "wrong retval")
			test.validateAcceptSchema(t, event)
		}, "test_accept_af_inet")
	})

	t.Run("accept-af-inet6-any-tcp-success-no-sockaddrin", func(t *testing.T) {
		if ebpfLessEnabled {
			t.Skip("Not available for ebpfLess")
		}

		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		port := rand.IntN(MAX-MIN) + MIN

		test.WaitSignalFromRule(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "accept", "AF_INET6", "::", "::1", strconv.Itoa(port), "false")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_accept_af_inet6")
			assert.Equal(t, "accept", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(unix.AF_INET6), event.Accept.AddrFamily, "wrong address family")
			assert.Equal(t, uint16(port), event.Accept.Addr.Port, "wrong address port")
			assert.Equal(t, "::1", event.Accept.Addr.IPNet.IP.String(), "wrong address")
			assert.LessOrEqual(t, int64(0), event.Accept.Retval, "wrong retval")
			test.validateAcceptSchema(t, event)
		}, "test_accept_af_inet6")
	})

	t.Run("accept-af-inet6-any-tcp-success-sockaddrin", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		port := rand.IntN(MAX-MIN) + MIN

		test.WaitSignalFromRule(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "accept", "AF_INET6", "::", "::1", strconv.Itoa(port), "true")
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_accept_af_inet6")
			assert.Equal(t, "accept", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(unix.AF_INET6), event.Accept.AddrFamily, "wrong address family")
			assert.Equal(t, "::1", event.Accept.Addr.IPNet.IP.String(), "wrong address")
			assert.LessOrEqual(t, int64(0), event.Accept.Retval, "wrong retval")
			test.validateAcceptSchema(t, event)
		}, "test_accept_af_inet6")
	})
}
