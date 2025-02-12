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

	"golang.org/x/net/nettest"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func udpCreateSocketAndSend(t *testing.T, sockDomain int, sockAddr syscall.Sockaddr, boundPort chan int, next chan struct{}, closed chan struct{}, errorExpected bool) {
	fd, err := syscall.Socket(sockDomain, syscall.SOCK_DGRAM, 0)
	if err != nil {
		close(boundPort)
		close(closed)
		t.Errorf("Socket error: %v", err)
		return
	}
	defer func() {
		_ = syscall.Close(fd)
		close(closed)
	}()

	// Message to send
	message := []byte("Hello, UDP receiver!")

	if err = syscall.Sendto(fd, message, 0, sockAddr); err != nil {
		if !errorExpected {
			close(boundPort)
			t.Errorf("SendTo error: %v", err)
			return
		}
	}

	sa, err := syscall.Getsockname(fd)
	if err != nil {
		close(boundPort)
		t.Errorf("Getsockname error: %v", err)
		return
	}
	switch addr := sa.(type) {
	case *syscall.SockaddrInet6:
		boundPort <- addr.Port
	case *syscall.SockaddrInet4:
		boundPort <- addr.Port
	default:
		close(boundPort)
		t.Error("Getsockname error: unknown Sockaddr type")
		return
	}
	<-next
}

func tcpCreateSocketAndSend(t *testing.T, sockDomain int, sockAddr syscall.Sockaddr, boundPort chan int, next chan struct{}, closed chan struct{}, errorExpected bool) {
	fd, err := syscall.Socket(sockDomain, syscall.SOCK_STREAM, 0)
	if err != nil {
		close(boundPort)
		close(closed)
		t.Errorf("Socket error: %v", err)
		return
	}
	defer func() {
		_ = syscall.Close(fd)
		close(closed)
	}()

	if err = syscall.Connect(fd, sockAddr); err != nil {
		if !errorExpected {
			close(boundPort)
			t.Errorf("Connect error: %v", err)
			return
		}
	}

	sa, err := syscall.Getsockname(fd)
	if err != nil {
		close(boundPort)
		t.Errorf("Getsockname error: %v", err)
		return
	}
	switch addr := sa.(type) {
	case *syscall.SockaddrInet6:
		boundPort <- addr.Port
	case *syscall.SockaddrInet4:
		boundPort <- addr.Port
	default:
		close(boundPort)
		t.Error("Getsockname error: unknown Sockaddr type")
		return
	}
	<-next
}

func createSocketAndSend(t *testing.T, sockDomain int, sockType int, sockAddr syscall.Sockaddr, boundPort chan int, next chan struct{}, closed chan struct{}, errorExpected bool) {
	switch sockType {
	case syscall.SOCK_STREAM:
		tcpCreateSocketAndSend(t, sockDomain, sockAddr, boundPort, next, closed, errorExpected)
	case syscall.SOCK_DGRAM:
		udpCreateSocketAndSend(t, sockDomain, sockAddr, boundPort, next, closed, errorExpected)
	default:
		t.Errorf("unknown socket type: %d", sockType)
	}
}

func TestFlowPidSecuritySKClassifyFlow(t *testing.T) {
	SkipIfNotAvailable(t)

	checkNetworkCompatibility(t)

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	ruleDefs := []*rules.RuleDefinition{
		// We use this dummy DNS rule to make sure the flow <-> pid tracking probes are loaded
		{
			ID:         "test_dns",
			Expression: `dns.question.name == "testsuite"`,
		},
	}

	t.Logf("host proc: %s", kernel.ProcFSRoot())
	pid := utils.Getpid()
	netns, err := getCurrentNetns()
	if err != nil {
		t.Fatalf("failed to get the network namespace: %v", err)
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("test_sock_ipv4_udp_send_127.0.0.1:1234", func(t *testing.T) {
		boundPort := make(chan int)
		next := make(chan struct{})
		closed := make(chan struct{})

		go createSocketAndSend(
			t,
			syscall.AF_INET,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet4{Port: 1234, Addr: [4]byte{127, 0, 0, 1}},
			boundPort,
			next,
			closed,
			false,
		)
		checkFlowPidEntry(
			t,
			test,
			FlowPid{
				Netns: netns,
				Port:  0, // resolved at runtime
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint32(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
			boundPort,
			next,
			closed,
			false,
		)
	})

	t.Run("test_sock_ipv6_udp_send_[::1]:2234", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		boundPort := make(chan int)
		next := make(chan struct{})
		closed := make(chan struct{})

		go createSocketAndSend(
			t,
			syscall.AF_INET6,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet6{Port: 2234, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			boundPort,
			next,
			closed,
			false,
		)
		checkFlowPidEntry(
			t,
			test,
			FlowPid{
				Netns: netns,
				Port:  0, // resolved at runtime
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint32(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
			boundPort,
			next,
			closed,
			false,
		)
	})

	t.Run("test_sock_ipv6_udp_send_127.0.0.1:3234", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		boundPort := make(chan int)
		next := make(chan struct{})
		closed := make(chan struct{})

		go createSocketAndSend(
			t,
			syscall.AF_INET6,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet4{Port: 3234, Addr: [4]byte{127, 0, 0, 1}},
			boundPort,
			next,
			closed,
			false,
		)
		checkFlowPidEntry(
			t,
			test,
			FlowPid{
				Netns: netns,
				Port:  0, // resolved at runtime
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint32(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
			boundPort,
			next,
			closed,
			false,
		)
	})
}
