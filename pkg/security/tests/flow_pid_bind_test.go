// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/nettest"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

var networkNamespacePattern = regexp.MustCompile(`net:\[(\d+)\]`)

func htons(port uint16) uint16 {
	return (port<<8)&0xFF00 | (port>>8)&0x00FF
}

func dumpMap(t *testing.T, m *ebpf.Map) {
	t.Log("Dumping flow_pid map ...")
	it := m.Iterate()
	a := FlowPid{}
	b := FlowPidEntry{}
	for it.Next(&a, &b) {
		t.Logf(" - key %+v value %+v", a, b)
	}
}

func getCurrentNetns() (uint32, error) {
	// open netns
	f, err := os.Open("/proc/self/ns/net")
	if err != nil {
		return 0, err
	}
	defer f.Close()

	l, err := os.Readlink(f.Name())
	if err != nil {
		return 0, err
	}

	matches := networkNamespacePattern.FindSubmatch([]byte(l))
	if len(matches) <= 1 {
		return 0, fmt.Errorf("couldn't parse network namespace ID: %s", l)
	}

	netns, err := strconv.ParseUint(string(matches[1]), 10, 32)
	if err != nil {
		return 0, err
	}
	return uint32(netns), nil
}

type FlowPid struct {
	Addr0    uint64
	Addr1    uint64
	Netns    uint32
	Port     uint16
	Protocol uint8 // l4_protocol
	Padding  uint8
}

type FlowPidEntry struct {
	OwnerSK   uint64
	Pid       uint32
	EntryType uint16
	Padding   uint16
}

func createSocketAndBind(t *testing.T, sockDomain int, sockType int, sockAddr syscall.Sockaddr, bound chan int, next chan struct{}, closed chan struct{}, errorExpected bool) {
	fd, err := syscall.Socket(sockDomain, sockType, 0)
	if err != nil {
		close(bound)
		close(closed)
		t.Errorf("Socket error: %v", err)
		return
	}
	defer func() {
		_ = syscall.Close(fd)
		close(closed)
	}()

	if err := syscall.Bind(fd, sockAddr); err != nil {
		if !errorExpected {
			close(bound)
			t.Errorf("Bind error: %v", err)
			return
		}
	}

	// retrieve bound port
	boundPort := 0
	if !errorExpected {
		sa, err := syscall.Getsockname(fd)
		if err != nil {
			close(bound)
			t.Errorf("Getsockname error: %v", err)
			return
		}
		switch addr := sa.(type) {
		case *syscall.SockaddrInet6:
			boundPort = addr.Port
		case *syscall.SockaddrInet4:
			boundPort = addr.Port
		default:
			close(bound)
			t.Error("Getsockname error: unknown Sockaddr type")
			return
		}
	}

	bound <- boundPort
	<-next
}

func checkBindFlowPidEntry(t *testing.T, testModule *testModule, key FlowPid, expectedEntry FlowPidEntry, closeClientSocket chan struct{}, clientSocketClosed chan struct{}, errorExpected bool) {
	// check that an entry exists for the newly bound server
	p, ok := testModule.probe.PlatformProbe.(*probe.EBPFProbe)
	if !ok {
		close(closeClientSocket)
		t.Skip("skipping non eBPF probe")
		return
	}

	m, _, err := p.Manager.GetMap("flow_pid")
	if err != nil {
		close(closeClientSocket)
		t.Errorf("failed to get map flow_pid: %v", err)
		return
	}

	value := FlowPidEntry{}
	if !errorExpected {
		if err := m.Lookup(&key, &value); err != nil {
			dumpMap(t, m)
			t.Errorf("Failed to lookup flow_pid: %v", err)
		} else {
			assert.Equal(t, expectedEntry.Pid, value.Pid, "wrong pid")
			assert.Equal(t, expectedEntry.EntryType, value.EntryType, "wrong entry type")
		}
	}

	close(closeClientSocket)

	// wait until the socket is closed and make sure the entry is no longer present
	<-clientSocketClosed
	if err := m.Lookup(&key, &value); err == nil {
		dumpMap(t, m)
		t.Errorf("flow_pid entry wasn't deleted: %+v", value)
	}
}

func TestFlowPidBind(t *testing.T) {
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

	t.Run("test_sock_ipv4_udp_bind_0.0.0.0:1234", func(t *testing.T) {
		boundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet4{Port: 1234, Addr: [4]byte{0, 0, 0, 0}},
			boundPort,
			closeClientSocket,
			clientSocketClosed,
			false,
		)
		checkBindFlowPidEntry(
			t,
			test,
			FlowPid{
				Netns:    netns,
				Port:     htons(uint16(<-boundPort)),
				Protocol: syscall.IPPROTO_UDP,
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
			closeClientSocket,
			clientSocketClosed,
			false,
		)
	})

	t.Run("test_sock_ipv4_udp_bind_127.0.0.1:1235", func(t *testing.T) {
		boundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet4{Port: 1235, Addr: [4]byte{127, 0, 0, 1}},
			boundPort,
			closeClientSocket,
			clientSocketClosed,
			false,
		)
		checkBindFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr0:    binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
				Netns:    netns,
				Port:     htons(uint16(<-boundPort)),
				Protocol: syscall.IPPROTO_UDP,
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
			closeClientSocket,
			clientSocketClosed,
			false,
		)
	})

	t.Run("test_sock_ipv4_udp_bind_127.0.0.1:0", func(t *testing.T) {
		boundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet4{Port: 0, Addr: [4]byte{127, 0, 0, 1}},
			boundPort,
			closeClientSocket,
			clientSocketClosed,
			false,
		)
		checkBindFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr0:    binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
				Netns:    netns,
				Port:     htons(uint16(<-boundPort)),
				Protocol: syscall.IPPROTO_UDP,
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
			closeClientSocket,
			clientSocketClosed,
			false,
		)
	})

	t.Run("test_sock_ipv6_udp_bind_[::]:1236", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		boundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET6,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet6{Port: 1236, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
			boundPort,
			closeClientSocket,
			clientSocketClosed,
			false,
		)
		checkBindFlowPidEntry(
			t,
			test,
			FlowPid{
				Netns:    netns,
				Port:     htons(uint16(<-boundPort)),
				Protocol: syscall.IPPROTO_UDP,
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
			closeClientSocket,
			clientSocketClosed,
			false,
		)
	})

	t.Run("test_sock_ipv6_udp_bind_[::1]:1237", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		boundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET6,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet6{Port: 1237, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			boundPort,
			closeClientSocket,
			clientSocketClosed,
			false,
		)
		checkBindFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr1:    binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
				Netns:    netns,
				Port:     htons(uint16(<-boundPort)),
				Protocol: syscall.IPPROTO_UDP,
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
			closeClientSocket,
			clientSocketClosed,
			false,
		)
	})

	t.Run("test_sock_ipv6_udp_bind_[::1]:0", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		boundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET6,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet6{Port: 0, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			boundPort,
			closeClientSocket,
			clientSocketClosed,
			false,
		)
		checkBindFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr1:    binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
				Netns:    netns,
				Port:     htons(uint16(<-boundPort)),
				Protocol: syscall.IPPROTO_UDP,
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
			closeClientSocket,
			clientSocketClosed,
			false,
		)
	})

	t.Run("test_sock_ipv4_tcp_bind_0.0.0.0:1234", func(t *testing.T) {
		boundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 1234, Addr: [4]byte{0, 0, 0, 0}},
			boundPort,
			closeClientSocket,
			clientSocketClosed,
			false,
		)
		checkBindFlowPidEntry(
			t,
			test,
			FlowPid{
				Netns:    netns,
				Port:     htons(uint16(<-boundPort)),
				Protocol: syscall.IPPROTO_TCP,
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
			closeClientSocket,
			clientSocketClosed,
			false,
		)
	})

	t.Run("test_sock_ipv4_tcp_bind_127.0.0.1:1235", func(t *testing.T) {
		boundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 1235, Addr: [4]byte{127, 0, 0, 1}},
			boundPort,
			closeClientSocket,
			clientSocketClosed,
			false,
		)
		checkBindFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr0:    binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
				Netns:    netns,
				Port:     htons(uint16(<-boundPort)),
				Protocol: syscall.IPPROTO_TCP,
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
			closeClientSocket,
			clientSocketClosed,
			false,
		)
	})

	t.Run("test_sock_ipv4_tcp_bind_127.0.0.1:0", func(t *testing.T) {
		boundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 0, Addr: [4]byte{127, 0, 0, 1}},
			boundPort,
			closeClientSocket,
			clientSocketClosed,
			false,
		)
		checkBindFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr0:    binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
				Netns:    netns,
				Port:     htons(uint16(<-boundPort)),
				Protocol: syscall.IPPROTO_TCP,
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
			closeClientSocket,
			clientSocketClosed,
			false,
		)
	})

	t.Run("test_sock_ipv6_tcp_bind_[::]:1236", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		boundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 1236, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
			boundPort,
			closeClientSocket,
			clientSocketClosed,
			false,
		)
		checkBindFlowPidEntry(
			t,
			test,
			FlowPid{
				Netns:    netns,
				Port:     htons(uint16(<-boundPort)),
				Protocol: syscall.IPPROTO_TCP,
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
			closeClientSocket,
			clientSocketClosed,
			false,
		)
	})

	t.Run("test_sock_ipv6_tcp_bind_[::1]:1237", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		boundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 1237, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			boundPort,
			closeClientSocket,
			clientSocketClosed,
			false,
		)
		checkBindFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr1:    binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
				Netns:    netns,
				Port:     htons(uint16(<-boundPort)),
				Protocol: syscall.IPPROTO_TCP,
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
			closeClientSocket,
			clientSocketClosed,
			false,
		)
	})

	t.Run("test_sock_ipv6_tcp_bind_[::1]:0", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		boundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 0, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			boundPort,
			closeClientSocket,
			clientSocketClosed,
			false,
		)
		checkBindFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr1:    binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
				Netns:    netns,
				Port:     htons(uint16(<-boundPort)),
				Protocol: syscall.IPPROTO_TCP,
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
			closeClientSocket,
			clientSocketClosed,
			false,
		)
	})
}

func TestFlowPidBindLeak(t *testing.T) {
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

	pid := utils.Getpid()
	netns, err := utils.NewNSPathFromPid(pid, utils.NetNsType).GetNSID()
	if err != nil {
		t.Fatalf("failed to get the network namespace: %v", err)
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("test_sock_ipv4_udp_bind_99.99.99.99:2234", func(t *testing.T) {
		boundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet4{Port: 2234, Addr: [4]byte{99, 99, 99, 99}},
			boundPort,
			closeClientSocket,
			clientSocketClosed,
			true,
		)

		select {
		case <-boundPort:
		case <-time.After(30 * time.Second):
			t.Fatal("Timeout waiting for test_sock_ipv4_udp_bind_99.99.99.99:2234")
		}
		checkBindFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr0:    binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 99, 99, 99, 99}),
				Netns:    netns,
				Port:     htons(2234),
				Protocol: syscall.IPPROTO_UDP,
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
			closeClientSocket,
			clientSocketClosed,
			true,
		)
	})

	t.Run("test_sock_ipv4_tcp_bind_99.99.99.99:2235", func(t *testing.T) {
		boundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 2235, Addr: [4]byte{99, 99, 99, 99}},
			boundPort,
			closeClientSocket,
			clientSocketClosed,
			true,
		)

		select {
		case <-boundPort:
		case <-time.After(30 * time.Second):
			t.Fatal("Timeout waiting for test_sock_ipv4_tcp_bind_99.99.99.99:2235")
		}
		checkBindFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr0:    binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 99, 99, 99, 99}),
				Netns:    netns,
				Port:     htons(2235),
				Protocol: syscall.IPPROTO_TCP,
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
			closeClientSocket,
			clientSocketClosed,
			true,
		)
	})

	t.Run("test_sock_ipv6_udp_bind_[99*]:2236", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		boundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET6,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet6{Port: 2236, Addr: [16]byte{99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99}},
			boundPort,
			closeClientSocket,
			clientSocketClosed,
			true,
		)
		select {
		case <-boundPort:
		case <-time.After(30 * time.Second):
			t.Fatal("Timeout waiting for test_sock_ipv6_udp_bind_[99*]:2236")
		}

		checkBindFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr0:    binary.BigEndian.Uint64([]byte{99, 99, 99, 99, 99, 99, 99, 99}),
				Addr1:    binary.BigEndian.Uint64([]byte{99, 99, 99, 99, 99, 99, 99, 99}),
				Netns:    netns,
				Port:     htons(2236),
				Protocol: syscall.IPPROTO_UDP,
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
			closeClientSocket,
			clientSocketClosed,
			true,
		)
	})

	t.Run("test_sock_ipv6_tcp_bind_[99*]:2237", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		boundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET6,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet6{Port: 2237, Addr: [16]byte{99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99}},
			boundPort,
			closeClientSocket,
			clientSocketClosed,
			true,
		)

		select {
		case <-boundPort:
		case <-time.After(30 * time.Second):
			t.Fatal("Timeout waiting for test_sock_ipv6_tcp_bind_[99*]:2237")
		}

		checkBindFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr0:    binary.BigEndian.Uint64([]byte{99, 99, 99, 99, 99, 99, 99, 99}),
				Addr1:    binary.BigEndian.Uint64([]byte{99, 99, 99, 99, 99, 99, 99, 99}),
				Netns:    netns,
				Port:     htons(2237),
				Protocol: syscall.IPPROTO_TCP,
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
			closeClientSocket,
			clientSocketClosed,
			true,
		)
		fmt.Println("FlowPidBindLeak test completed successfully")
	})
}
func TestMultipleProtocols(t *testing.T) {
	SkipIfNotAvailable(t)
	tcpbindReady := make(chan int, 1)
	tcplistenReady := make(chan struct{}, 1)
	udpbindReady := make(chan int, 1)
	udpwaitReady := make(chan struct{}, 1)
	udpCloseReady := make(chan struct{}, 1)
	tcpCloseReady := make(chan struct{}, 1)
	checkNetworkCompatibility(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "bind_multiple_udp",
			Expression: `bind.addr.family == AF_INET && bind.protocol == 17 && bind.addr.port == 2663`,
		},
		{
			ID:         "bind_multiple_tcp",
			Expression: `bind.addr.family == AF_INET && bind.protocol == 6 && bind.addr.port == 2663`,
		},
		// This rule is used to ensure that the flow <-> pid tracking probes are loaded
		{
			ID:         "test_dns",
			Expression: `dns.question.name == "testsuite"`,
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

	t.Run("bind-udp-and-tcp-on-same-port", func(t *testing.T) {
		//  --- TCP BIND ---
		var tempTCPPid int

		test.WaitSignalFromRule(t, func() error {
			go func() {
				timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				cmd := exec.CommandContext(timeoutCtx, syscallTester, "bind-and-listen", "2663", "tcp")

				stdout, err := cmd.StdoutPipe()
				if err != nil {
					t.Errorf("TCP: failed to get stdout pipe: %v", err)
					return
				}
				stderr, err := cmd.StderrPipe()
				if err != nil {
					fmt.Fprintf(os.Stderr, "unable to start StderrPipe: %s", err)
					return
				}

				errscanner := bufio.NewScanner(stderr)
				go func() {
					for errscanner.Scan() {
						fmt.Printf("[TCP STDERR] %s\n", errscanner.Text())
						if err := errscanner.Err(); err != nil {
							t.Errorf("TCP: error reading stderr: %v", err)
						}
					}
				}()

				if err := cmd.Start(); err != nil {
					t.Errorf("TCP: failed to start syscall_tester: %v", err)
					return
				}

				scanner := bufio.NewScanner(stdout)
				if err := scanner.Err(); err != nil {
					t.Errorf("TCP: failed to read stdout: %v", err)
					return
				}
				for scanner.Scan() {
					line := scanner.Text()
					if strings.HasPrefix(line, "PID: ") {
						pidStr := strings.TrimPrefix(line, "PID: ")
						pid, err := strconv.Atoi(pidStr)
						if err == nil {
							tcpbindReady <- pid // Synchro on PID
						}
					}
					if strings.HasPrefix(line, "Listening on port") {
						tcplistenReady <- struct{}{} // Synchro on listen ready
					}
					if strings.HasPrefix(line, "Closing socket...") {
						tcpCloseReady <- struct{}{} // Synchro on close ready
					}
				}
				defer cmd.Wait()
			}()
			return nil
		}, func(_ *model.Event, _ *rules.Rule) {
		}, "bind_multiple_tcp")

		// --- UDP BIND ---
		var tempUDPPid int

		test.WaitSignalFromRule(t, func() error {
			go func() {
				timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				cmd := exec.CommandContext(timeoutCtx, syscallTester, "bind-and-listen", "2663", "udp")

				stdout, err := cmd.StdoutPipe()
				if err != nil {
					t.Errorf("UDP: failed to get stdout pipe: %v", err)
					return
				}
				stderr, err := cmd.StderrPipe()
				if err != nil {
					fmt.Fprintf(os.Stderr, "unable to start StderrPipe: %s", err)
					return
				}
				errscanner := bufio.NewScanner(stderr)
				go func() {
					for errscanner.Scan() {
						fmt.Printf("[UDP STDERR] %s\n", errscanner.Text())
						if err := errscanner.Err(); err != nil {
							t.Errorf("UDP: error reading stderr: %v", err)
						}
					}
				}()

				if err := cmd.Start(); err != nil {
					t.Errorf("UDP: failed to start syscall_tester: %v", err)
					return
				}

				scanner := bufio.NewScanner(stdout)
				if err := scanner.Err(); err != nil {
					t.Errorf("UDP: failed to read stdout: %v", err)
					return
				}
				for scanner.Scan() {
					line := scanner.Text()
					if strings.HasPrefix(line, "PID: ") {
						pidStr := strings.TrimPrefix(line, "PID: ")
						pid, err := strconv.Atoi(pidStr)
						if err == nil {
							udpbindReady <- pid // Synchro on PID
						}
					}
					if strings.HasPrefix(line, "Waiting on port") {
						udpwaitReady <- struct{}{} // Synchro on wait ready
					}
					if strings.HasPrefix(line, "Closing socket...") {
						udpCloseReady <- struct{}{} // Synchro on close ready
					}
				}
				defer cmd.Wait()
			}()
			return nil
		}, func(_ *model.Event, _ *rules.Rule) {

		}, "bind_multiple_udp")

		//  --- TEST ---
		// Wait for both TCP and UDP bind to be ready
		select {
		case tempTCPPid = <-tcpbindReady:
		case <-time.After(30 * time.Second):
			t.Fatal("Timeout waiting for TCP PID in MultipleProtocols test")
		}

		select {
		case tempUDPPid = <-udpbindReady:
		case <-time.After(30 * time.Second):
			t.Fatal("Timeout waiting for UDP PID in MultipleProtocols test")
		}

		p, ok := test.probe.PlatformProbe.(*probe.EBPFProbe)
		if !ok {
			t.Skip("skipping non eBPF probe")
			return
		}

		m, _, err := p.Manager.GetMap("flow_pid")
		if err != nil {
			t.Fatalf("failed to get map flow_pid: %v", err)
		}
		// Get values in the map

		netns, err := getCurrentNetns() // the netns in the map will be different if syscall_tester is executed in a separated container
		if err != nil {
			t.Fatalf("failed to get current netns: %v", err)
		}

		expectedPort := uint16(2663)
		htonsPort := htons(expectedPort)

		tcpKey := FlowPid{
			Netns:    netns,
			Port:     htonsPort,
			Protocol: uint8(unix.IPPROTO_TCP),
		}
		udpKey := FlowPid{
			Netns:    netns,
			Port:     htonsPort,
			Protocol: uint8(unix.IPPROTO_UDP),
		}
		var tcpVal = FlowPidEntry{}
		var udpVal = FlowPidEntry{}
		if err := m.Lookup(&tcpKey, &tcpVal); err != nil {
			dumpMap(t, m)
			t.Errorf("TCP entry not found for key: %+v, error: %v", tcpKey, err)
		}

		if err := m.Lookup(&udpKey, &udpVal); err != nil {
			dumpMap(t, m)
			t.Errorf("UDP entry not found for key: %+v, error: %v", udpKey, err)
		}

		// Check PIDs to make tests work for both docker and non docker tests
		var tcpPid, udpPid uint32
		if uint32(tempTCPPid) == tcpVal.Pid {
			// We are in non docker tests
			tcpPid = uint32(tempTCPPid)
			udpPid = uint32(tempUDPPid)
		} else {
			// We might be in docker tests
			// Discover syscall_tester processes from /host/proc
			var discoveredPIDs []uint32
			procDir := "/host/proc"
			entries, err := os.ReadDir(procDir)
			fmt.Printf("Entries are %v\n", entries)
			if err != nil {
				t.Logf("failed to read %s: %v", procDir, err)
			} else {
				for _, entry := range entries {
					if !entry.IsDir() {
						continue
					}
					pidInt, err := strconv.Atoi(entry.Name())
					if err != nil {
						continue
					}
					commPath := filepath.Join(procDir, entry.Name(), "comm")
					data, err := os.ReadFile(commPath)
					if err != nil {
						continue
					}
					if strings.Contains(string(data), "syscall_tester") {
						discoveredPIDs = append(discoveredPIDs, uint32(pidInt))
					}
				}
			}

			if len(discoveredPIDs) != 2 {
				t.Logf("expected 2 syscall_tester processes, found %d: %v", len(discoveredPIDs), discoveredPIDs)
			} else {
				if discoveredPIDs[0] == tcpVal.Pid && discoveredPIDs[1] == udpVal.Pid {
					// First try: associate discoveredPIDs[0] with TCP and discoveredPIDs[1] with UDP.
					tcpPid = discoveredPIDs[0]
					udpPid = discoveredPIDs[1]
				} else if discoveredPIDs[0] == udpVal.Pid && discoveredPIDs[1] == tcpVal.Pid {
					// Second try (swap): assume discoveredPIDs[1] is for TCP and discoveredPIDs[0] for UDP.
					tcpPid = discoveredPIDs[1]
					udpPid = discoveredPIDs[0]
				} else {
					t.Errorf("unexpected PIDs found: %v, tcpVal.Pid: %d, udpVal.Pid: %d", discoveredPIDs, tcpVal.Pid, udpVal.Pid)
				}
			}

		}

		assert.NotEqual(t, tcpVal.Pid, udpVal.Pid, "TCP and UDP should be from different PIDs")
		assert.Equal(t, tcpPid, tcpVal.Pid, "TCP PID mismatch")
		assert.Equal(t, udpPid, udpVal.Pid, "UDP PID mismatch")
		assert.Equal(t, uint16(0), tcpVal.EntryType, "TCP entry type mismatch")
		assert.Equal(t, uint16(0), udpVal.EntryType, "UDP entry type mismatch")
		assert.Equal(t, uint8(unix.IPPROTO_TCP), tcpKey.Protocol, "TCP protocol mismatch")
		assert.Equal(t, uint8(unix.IPPROTO_UDP), udpKey.Protocol, "UDP protocol mismatch")
		assert.Equal(t, htonsPort, tcpKey.Port, "TCP port mismatch")
		assert.Equal(t, htonsPort, udpKey.Port, "UDP port mismatch")

		// Close sockets
		// Wait for both TCP and UDP listen/wait to be ready
		select {
		case <-tcplistenReady:
		case <-time.After(30 * time.Second):
			t.Fatal("Timeout waiting for TCP listen in MultipleProtocols test")
		}
		select {
		case <-udpwaitReady:
		case <-time.After(30 * time.Second):
			t.Fatal("Timeout waiting for UDP listen in MultipleProtocols test")
		}

		time.Sleep(1 * time.Second)
		if connTCP, err := net.Dial("tcp", "127.0.0.1:2663"); err != nil {
			t.Errorf("failed to connect to TCP socket: %v", err)
		} else {
			_, _ = connTCP.Write([]byte("CLOSE\n"))
			_ = connTCP.Close()
		}
		if connUDP, err := net.Dial("udp", "127.0.0.1:2663"); err != nil {
			t.Errorf("failed to connect to UDP socket: %v", err)
		} else {
			_, _ = connUDP.Write([]byte("CLOSE\n"))
			_ = connUDP.Close()
		}
		// Check that entries are removed
		select {
		case <-tcpCloseReady:
		case <-time.After(30 * time.Second):
			t.Fatal("Timeout waiting for TCP close in MultipleProtocols test")
		}
		select {
		case <-udpCloseReady:
		case <-time.After(30 * time.Second):
			t.Fatal("Timeout waiting for UDP close in MultipleProtocols test")
		}

		time.Sleep(1 * time.Second)
		if err := m.Lookup(&tcpKey, &tcpVal); err == nil {
			dumpMap(t, m)
			t.Errorf("flow_pid entry wasn't deleted: %+v", tcpVal)
		}
		if err := m.Lookup(&udpKey, &udpVal); err == nil {
			dumpMap(t, m)
			t.Errorf("flow_pid entry wasn't deleted: %+v", udpVal)
		}
	})
}
