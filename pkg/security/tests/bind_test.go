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
	"net"
	"os/exec"
	"syscall"
	"testing"

	"github.com/iceber/iouring-go"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestBindEvent(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_bind_af_inet",
			Expression: `bind.addr.family == AF_INET && process.file.name in [ "syscall_tester", "testsuite" ]`,
		},
		{
			ID:         "test_bind_af_inet6",
			Expression: `bind.addr.family == AF_INET6 && process.file.name == "syscall_tester"`,
		},
		{
			ID:         "test_bind_af_unix",
			Expression: `bind.addr.family == AF_UNIX && process.file.name == "syscall_tester"`,
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

	test.Run(t, "bind-af-inet-any-success-tcp", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{"bind", "AF_INET", "any", "tcp"}
		envs := []string{}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc(syscallTester, args, envs)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}

			return nil
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "bind", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(unix.AF_INET), event.Bind.AddrFamily, "wrong address family")
			assert.Equal(t, uint16(4242), event.Bind.Addr.Port, "wrong address port")
			assert.Equal(t, string("0.0.0.0/32"), event.Bind.Addr.IPNet.String(), "wrong address")
			assert.Equal(t, int64(0), event.Bind.Retval, "wrong retval")
			assert.Equal(t, uint16(unix.IPPROTO_TCP), event.Bind.Protocol, "wrong protocol")

			test.validateBindSchema(t, event)
		})
	})

	test.Run(t, "bind-af-inet-any-success-tcp-io-uring", func(t *testing.T, _ wrapperType, _ func(cmd string, args []string, envs []string) *exec.Cmd) {
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
			Port: 4242,
			Addr: [4]byte(net.IPv4(0, 0, 0, 0)),
		}

		prepRequest, err := iouring.Bind(int32(fd), sa)
		if err != nil {
			t.Fatal(err)
		}

		ch := make(chan iouring.Result, 1)

		test.WaitSignal(t, func() error {
			if _, err = iour.SubmitRequest(prepRequest, ch); err != nil {
				return err
			}

			result := <-ch
			ret, err := result.ReturnInt()
			if err != nil {
				if err == syscall.EBADF || err == syscall.EINVAL {
					return ErrSkipTest{fmt.Sprintf("bind not supported by io_uring: %s", err)}
				}
				return err
			}

			if ret < 0 {
				return fmt.Errorf("failed to bind with io_uring: %d", ret)
			}

			return err
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "bind", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(unix.AF_INET), event.Bind.AddrFamily, "wrong address family")
			assert.Equal(t, uint16(4242), event.Bind.Addr.Port, "wrong address port")
			assert.Equal(t, string("0.0.0.0/32"), event.Bind.Addr.IPNet.String(), "wrong address")
			assert.Equal(t, int64(0), event.Bind.Retval, "wrong retval")
			assert.Equal(t, uint16(unix.IPPROTO_TCP), event.Bind.Protocol, "wrong protocol")

			test.validateBindSchema(t, event)
		})
	})

	test.Run(t, "bind-af-inet-any-success-udp", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{"bind", "AF_INET", "any", "udp"}
		envs := []string{}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc(syscallTester, args, envs)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}

			return nil
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "bind", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(unix.AF_INET), event.Bind.AddrFamily, "wrong address family")
			assert.Equal(t, uint16(4242), event.Bind.Addr.Port, "wrong address port")
			assert.Equal(t, string("0.0.0.0/32"), event.Bind.Addr.IPNet.String(), "wrong address")
			assert.Equal(t, int64(0), event.Bind.Retval, "wrong retval")
			assert.Equal(t, uint16(unix.IPPROTO_UDP), event.Bind.Protocol, "wrong protocol")

			test.validateBindSchema(t, event)
		})
	})

	test.Run(t, "bind-af-inet6-any-success", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{"bind", "AF_INET6", "any"}
		envs := []string{}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc(syscallTester, args, envs)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}

			return nil
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "bind", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(unix.AF_INET6), event.Bind.AddrFamily, "wrong address family")
			assert.Equal(t, uint16(4242), event.Bind.Addr.Port, "wrong address port")
			assert.Equal(t, string("::/128"), event.Bind.Addr.IPNet.String(), "wrong address")
			assert.Equal(t, int64(0), event.Bind.Retval, "wrong retval")

			test.validateBindSchema(t, event)
		})
	})

	test.Run(t, "bind-af-unknown-unix", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{"bind", "AF_UNIX"}
		envs := []string{}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc(syscallTester, args, envs)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}

			return nil
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "bind", event.GetType(), "wrong event type")
			assert.Equal(t, uint16(unix.AF_UNIX), event.Bind.AddrFamily, "wrong address family")
			assert.Equal(t, uint16(0), event.Bind.Addr.Port, "wrong address port")
			assert.Equal(t, net.IPNet{IP: net.IP(nil), Mask: net.IPMask(nil)},
				event.Bind.Addr.IPNet, "wrong address")
			assert.Equal(t, int64(0), event.Bind.Retval, "wrong retval")

			test.validateBindSchema(t, event)
		})
	})
}
