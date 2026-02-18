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
	"net"
	"net/http"
	"syscall"
	"testing"
	"time"

	"github.com/iceber/iouring-go"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/nettest"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestConnectEventAFIntetTCP(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_connect_af_inet",
			Expression: `connect.addr.port == 4242 && connect.addr.family == AF_INET && process.file.name == "syscall_tester"`,
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
	listener, err := net.Listen("tcp", ":4242")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go listener.Accept()

	test.WaitSignalFromRule(t, func() error {
		if err := runSyscallTesterFunc(context.Background(), t, syscallTester, "connect", "AF_INET", "any", "tcp", "4242"); err != nil {
			return err
		}
		return err
	}, func(event *model.Event, _ *rules.Rule) {
		assert.Equal(t, "connect", event.GetType(), "wrong event type")
		assert.Equal(t, uint16(unix.AF_INET), event.Connect.AddrFamily, "wrong address family")
		assert.Equal(t, uint16(4242), event.Connect.Addr.Port, "wrong address port")
		assert.Equal(t, string("0.0.0.0/32"), event.Connect.Addr.IPNet.String(), "wrong address")
		assert.Equal(t, uint16(unix.IPPROTO_TCP), event.Connect.Protocol, "wrong protocol")
		assert.Equal(t, int64(0), event.Connect.Retval, "wrong retval")
		test.validateConnectSchema(t, event)
	}, "test_connect_af_inet")
}
func TestConnectEventAFInetIOUring(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_connect_af_inet_io_uring",
			Expression: `connect.addr.port == 4243 && connect.addr.family == AF_INET && process.file.name == "testsuite"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	listener, err := net.Listen("tcp", ":4243")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go listener.Accept()

	// Wait 1 sec to ensure the listener is ready
	time.Sleep(1 * time.Second)

	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, unix.IPPROTO_TCP)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)

	iour, err := iouring.New(1)
	if err != nil {
		if errors.Is(err, unix.ENOTSUP) {
			t.Fatal(err)
		}
		t.Skip("io_uring not supported")
	}
	defer iour.Close()

	sa := unix.SockaddrInet4{
		Port: 4243,
		Addr: [4]byte(net.IPv4(0, 0, 0, 0)),
	}

	prepRequest, err := iouring.Connect(int32(fd), sa)
	if err != nil {
		t.Fatal(err)
	}
	ch := make(chan iouring.Result, 1)

	test.WaitSignalFromRule(t, func() error {
		if _, err = iour.SubmitRequest(prepRequest, ch); err != nil {
			return err
		}
		var result iouring.Result
		select {
		case result = <-ch:
		case <-time.After(8 * time.Second):
			return errors.New("timeout waiting for io_uring connect")
		}
		ret, err := result.ReturnInt()
		if err != nil {
			if err == syscall.EBADF || err == syscall.EINVAL {
				return ErrSkipTest{fmt.Sprintf("connect not supported by io_uring: %s", err)}
			}
			return err
		}

		if ret < 0 {
			return fmt.Errorf("failed to connect with io_uring: %d", ret)
		}

		return err
	}, func(event *model.Event, rule *rules.Rule) {
		assert.Equal(t, "connect", event.GetType(), "wrong event type")
		assertTriggeredRule(t, rule, "test_connect_af_inet_io_uring")
		assert.Equal(t, uint16(unix.AF_INET), event.Connect.AddrFamily, "wrong address family")
		assert.Equal(t, "testsuite", event.ProcessContext.FileEvent.BasenameStr, "wrong process name")
		assert.Equal(t, uint16(4243), event.Connect.Addr.Port, "wrong address port")
		assert.Equal(t, string("0.0.0.0/32"), event.Connect.Addr.IPNet.String(), "wrong address")
		assert.Equal(t, uint16(unix.IPPROTO_TCP), event.Connect.Protocol, "wrong protocol")
		assert.Contains(t, []int64{0, -int64(syscall.EAGAIN)}, event.Connect.Retval, "wrong retval")
		test.validateConnectSchema(t, event)
	}, "test_connect_af_inet_io_uring")
}
func TestConnectEventAFInetAnyUDP(t *testing.T) {
	SkipIfNotAvailable(t)
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_connect_af_inet",
			Expression: `connect.addr.port == 4244 && connect.addr.family == AF_INET && process.file.name == "syscall_tester"`,
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

	conn, err := net.ListenPacket("udp", ":4244")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	test.WaitSignalFromRule(t, func() error {
		if err := runSyscallTesterFunc(context.Background(), t, syscallTester, "connect", "AF_INET", "any", "udp", "4244"); err != nil {
			return err
		}
		return err
	}, func(event *model.Event, _ *rules.Rule) {
		assert.Equal(t, "connect", event.GetType(), "wrong event type")
		assert.Equal(t, uint16(unix.AF_INET), event.Connect.AddrFamily, "wrong address family")
		assert.Equal(t, uint16(4244), event.Connect.Addr.Port, "wrong address port")
		assert.Equal(t, string("0.0.0.0/32"), event.Connect.Addr.IPNet.String(), "wrong address")
		assert.Equal(t, uint16(unix.IPPROTO_UDP), event.Connect.Protocol, "wrong protocol")
		assert.Equal(t, int64(0), event.Connect.Retval, "wrong retval")
		test.validateConnectSchema(t, event)
	}, "test_connect_af_inet")
}
func TestConnectEventAFInet6AnyTCP(t *testing.T) {
	SkipIfNotAvailable(t)
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_connect_af_inet6",
			Expression: `connect.addr.port == 4245 && connect.addr.family == AF_INET6 && process.file.name == "syscall_tester"`,
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

	if !nettest.SupportsIPv6() {
		t.Skip("IPv6 is not supported")
	}

	listener, err := net.Listen("tcp", ":4245")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go listener.Accept()

	test.WaitSignalFromRule(t, func() error {
		if err := runSyscallTesterFunc(context.Background(), t, syscallTester, "connect", "AF_INET6", "any", "tcp", "4245"); err != nil {
			return err
		}
		return err
	}, func(event *model.Event, _ *rules.Rule) {
		assert.Equal(t, "connect", event.GetType(), "wrong event type")
		assert.Equal(t, uint16(unix.AF_INET6), event.Connect.AddrFamily, "wrong address family")
		assert.Equal(t, uint16(4245), event.Connect.Addr.Port, "wrong address port")
		assert.Equal(t, string("::/128"), event.Connect.Addr.IPNet.String(), "wrong address")
		assert.Equal(t, uint16(unix.IPPROTO_TCP), event.Connect.Protocol, "wrong protocol")
		assert.Equal(t, int64(0), event.Connect.Retval, "wrong retval")
		test.validateConnectSchema(t, event)
	}, "test_connect_af_inet6")
}
func TestConnectEventAFInet6AnyUDP(t *testing.T) {
	SkipIfNotAvailable(t)
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_connect_af_inet6",
			Expression: `connect.addr.port == 4246 && connect.addr.family == AF_INET6 && process.file.name == "syscall_tester"`,
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

	if !nettest.SupportsIPv6() {
		t.Skip("IPv6 is not supported")
	}

	conn, err := net.ListenPacket("udp", ":4246")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	test.WaitSignalFromRule(t, func() error {
		if err := runSyscallTesterFunc(context.Background(), t, syscallTester, "connect", "AF_INET6", "any", "udp", "4246"); err != nil {
			return err
		}
		return err
	}, func(event *model.Event, _ *rules.Rule) {
		assert.Equal(t, "connect", event.GetType(), "wrong event type")
		assert.Equal(t, uint16(unix.AF_INET6), event.Connect.AddrFamily, "wrong address family")
		assert.Equal(t, uint16(4246), event.Connect.Addr.Port, "wrong address port")
		assert.Equal(t, string("::/128"), event.Connect.Addr.IPNet.String(), "wrong address")
		assert.Equal(t, uint16(unix.IPPROTO_UDP), event.Connect.Protocol, "wrong protocol")
		assert.Equal(t, int64(0), event.Connect.Retval, "wrong retval")
		test.validateConnectSchema(t, event)
	}, "test_connect_af_inet6")
}
func TestConnectEventinetNonBlocking(t *testing.T) {
	SkipIfNotAvailable(t)
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_connect_nonblocking_socket",
			Expression: `connect.addr.port == 443 && process.file.name == "testsuite"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	test.WaitSignalFromRule(t, func() error {
		resp, err := http.Get("https://www.google.com")
		if err != nil {
			return err
		}
		resp.Body.Close()

		return nil
	}, func(event *model.Event, _ *rules.Rule) {
		assert.Equal(t, "connect", event.GetType(), "wrong event type")
		assert.Equal(t, uint16(443), event.Connect.Addr.Port, "wrong address port")
		test.validateConnectSchema(t, event)
	}, "test_connect_nonblocking_socket")
}
