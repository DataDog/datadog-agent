// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"testing"

	iouring "github.com/iceber/iouring-go"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestSocketEvent(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_socket_af_inet_tcp",
			Expression: `socket.domain == AF_INET && socket.type == SOCK_STREAM && socket.protocol == IPPROTO_TCP && process.file.name in [ "syscall_tester", "testsuite" ]`,
		},
		{
			ID:         "test_socket_af_inet_udp",
			Expression: `socket.domain == AF_INET && socket.type == SOCK_DGRAM && socket.protocol == IPPROTO_UDP && process.file.name in [ "syscall_tester", "testsuite" ]`,
		},
		{
			ID:         "test_socket_af_unix_stream",
			Expression: `socket.domain == AF_UNIX && socket.type == SOCK_STREAM && process.file.name in [ "syscall_tester", "testsuite" ]`,
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

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("socket-af-inet-tcp", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
			if err != nil {
				return err
			}
			syscall.Close(fd)
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_socket_af_inet_tcp")
			assert.Equal(t, "socket", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(syscall.AF_INET), event.Socket.Domain, "wrong socket domain")
			assert.Equal(t, uint16(syscall.SOCK_STREAM), event.Socket.Type, "wrong socket type")
			assert.Equal(t, uint16(syscall.IPPROTO_TCP), event.Socket.Protocol, "wrong socket protocol")
			assert.Greater(t, event.Socket.Retval, int64(0), "socket retval should be a valid fd")

			test.validateSocketSchema(t, event)
		}, "test_socket_af_inet_tcp")
	})

	t.Run("socket-af-inet-udp", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, syscall.IPPROTO_UDP)
			if err != nil {
				return err
			}
			syscall.Close(fd)
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_socket_af_inet_udp")
			assert.Equal(t, "socket", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(syscall.AF_INET), event.Socket.Domain, "wrong socket domain")
			assert.Equal(t, uint16(syscall.SOCK_DGRAM), event.Socket.Type, "wrong socket type")
			assert.Equal(t, uint16(syscall.IPPROTO_UDP), event.Socket.Protocol, "wrong socket protocol")
			assert.Greater(t, event.Socket.Retval, int64(0), "socket retval should be a valid fd")

			test.validateSocketSchema(t, event)
		}, "test_socket_af_inet_udp")
	})

	t.Run("socket-af-unix-stream", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
			if err != nil {
				return err
			}
			syscall.Close(fd)
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_socket_af_unix_stream")
			assert.Equal(t, "socket", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(syscall.AF_UNIX), event.Socket.Domain, "wrong socket domain")
			assert.Equal(t, uint16(syscall.SOCK_STREAM), event.Socket.Type, "wrong socket type")
			assert.Equal(t, uint16(0), event.Socket.Protocol, "socket protocol should be 0 (default for AF_UNIX)")
			assert.Greater(t, event.Socket.Retval, int64(0), "socket retval should be a valid fd")

			test.validateSocketSchema(t, event)
		}, "test_socket_af_unix_stream")
	})

	t.Run("socket-af-inet-raw-icmp", func(t *testing.T) {
		if ebpfLessEnabled {
			t.Skip("Not available for ebpfLess")
		}

		test.WaitSignalFromRule(t, func() error {
			fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_ICMP)
			if err != nil {
				// Raw sockets require CAP_NET_RAW; skip rather than fail when unavailable.
				if err == syscall.EPERM {
					t.Skipf("Skipping raw socket test: permission denied (need root privileges)")
					return nil
				}
				return err
			}
			syscall.Close(fd)
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_socket_af_inet_raw_icmp")
			assert.Equal(t, "socket", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(syscall.AF_INET), event.Socket.Domain, "wrong socket domain")
			assert.Equal(t, uint16(syscall.SOCK_RAW), event.Socket.Type, "wrong socket type")
			assert.Equal(t, uint16(syscall.IPPROTO_ICMP), event.Socket.Protocol, "wrong socket protocol")
			assert.Greater(t, event.Socket.Retval, int64(0), "socket retval should be a valid fd")

			test.validateSocketSchema(t, event)
		}, "test_socket_af_inet_raw_icmp")
	})

	t.Run("socket-af-inet-tcp-io-uring", func(t *testing.T) {
		// io_uring is unsupported under ebpfless; the exact exclude entry in
		// main_linux.go only takes effect if this subtest calls SkipIfNotAvailable.
		SkipIfNotAvailable(t)

		checkKernelCompatibility(t, "io_uring socket needs Linux 5.19", func(kv *kernel.Version) bool {
			return kv.Code < kernel.Kernel5_19
		})

		iour, err := iouring.New(1)
		if err != nil {
			if errors.Is(err, unix.ENOTSUP) {
				t.Fatal(err)
			}
			t.Skip("io_uring not supported")
		}
		defer iour.Close()

		prepRequest := ioUringPrepSocket(unix.AF_INET, unix.SOCK_STREAM, unix.IPPROTO_TCP)
		ch := make(chan iouring.Result, 1)

		test.WaitSignalFromRule(t, func() error {
			if _, err = iour.SubmitRequest(prepRequest, ch); err != nil {
				return err
			}

			result := <-ch
			fd, err := ioUringResult(result)
			if err != nil {
				return fmt.Errorf("io_uring error: %w", err)
			}

			if fd < 0 {
				// On a supported kernel a negative result is a real failure, not a skip:
				// a malformed SQE would also return a negative errno and hide the gap.
				return fmt.Errorf("failed to create socket with io_uring: %d", fd)
			}

			return unix.Close(fd)
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_socket_af_inet_tcp")
			assert.Equal(t, "socket", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(unix.AF_INET), event.Socket.Domain, "wrong socket domain")
			assert.Equal(t, uint16(unix.SOCK_STREAM), event.Socket.Type, "wrong socket type")
			assert.Equal(t, uint16(unix.IPPROTO_TCP), event.Socket.Protocol, "wrong socket protocol")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, true, value.(bool), "io_uring socket event should be async")

			test.validateSocketSchema(t, event)
		}, "test_socket_af_inet_tcp")
	})

	test.RunMultiMode(t, "socket-af-inet-tcp-syscall-tester", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		test.WaitSignalFromRule(t, func() error {
			cmd := cmdFunc(syscallTester, []string{"socket", "AF_INET", "SOCK_STREAM", "IPPROTO_TCP"}, nil)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_socket_af_inet_tcp")
			assert.Equal(t, "socket", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(syscall.AF_INET), event.Socket.Domain, "wrong socket domain")
			assert.Equal(t, uint16(syscall.SOCK_STREAM), event.Socket.Type, "wrong socket type")
			assert.Equal(t, uint16(syscall.IPPROTO_TCP), event.Socket.Protocol, "wrong socket protocol")

			test.validateSocketSchema(t, event)
		}, "test_socket_af_inet_tcp")
	})
}
