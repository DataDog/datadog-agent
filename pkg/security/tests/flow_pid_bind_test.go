// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"os/exec"
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
	netns, err := utils.NetNSPathFromPid(pid).GetProcessNetworkNamespace()
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

		<-boundPort

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

		<-boundPort

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

		<-boundPort

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

		<-boundPort

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
			Expression: `bind.addr.family == AF_INET && bind.protocol == 17 && bind.addr.port == 2236`,
		},
		{
			ID:         "bind_multiple_tcp",
			Expression: `bind.addr.family == AF_INET && bind.protocol == 6 && bind.addr.port == 2236`,
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

	test.Run(t, "bind-udp-and-tcp-on-same-port", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		//  --- TCP BIND ---
		var tcpPid int

		test.WaitSignal(t, func() error {
			go func() {
				args := []string{"bind-and-listen", "2236", "tcp"}
				cmd := cmdFunc(syscallTester, args, nil)

				stdout, err := cmd.StdoutPipe()
				if err != nil {
					t.Errorf("TCP: failed to get stdout pipe: %v", err)
					return
				}

				if err := cmd.Start(); err != nil {
					t.Errorf("TCP: failed to start syscall_tester: %v", err)
					return
				}

				scanner := bufio.NewScanner(stdout)
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
				_ = cmd.Wait()
			}()
			return nil
		}, func(_ *model.Event, _ *rules.Rule) {
		})

		// --- UDP BIND ---
		var udpPid int

		test.WaitSignal(t, func() error {
			go func() {
				args := []string{"bind-and-listen", "2236", "udp"}
				cmd := cmdFunc(syscallTester, args, nil)

				stdout, err := cmd.StdoutPipe()
				if err != nil {
					t.Errorf("UDP: failed to get stdout pipe: %v", err)
					return
				}

				if err := cmd.Start(); err != nil {
					t.Errorf("UDP: failed to start syscall_tester: %v", err)
					return
				}

				scanner := bufio.NewScanner(stdout)
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
				_ = cmd.Wait()
			}()
			return nil
		}, func(_ *model.Event, _ *rules.Rule) {

		})

		//  --- TEST ---
		// Wait for both TCP and UDP bind to be ready
		tcpPid = <-tcpbindReady
		udpPid = <-udpbindReady

		p, ok := test.probe.PlatformProbe.(*probe.EBPFProbe)
		if !ok {
			t.Skip("skipping non eBPF probe")
			return
		}

		m, _, err := p.Manager.GetMap("flow_pid")
		if err != nil {
			t.Fatalf("failed to get map flow_pid: %v", err)
		}

		netns, err := getCurrentNetns()
		if err != nil {
			t.Fatalf("failed to get current netns: %v", err)
		}

		expectedPort := uint16(2236)
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

		assert.NotEqual(t, tcpVal.Pid, udpVal.Pid, "TCP and UDP should be from different PIDs")
		assert.Equal(t, uint32(tcpPid), tcpVal.Pid, "TCP PID mismatch")
		assert.Equal(t, uint32(udpPid), udpVal.Pid, "UDP PID mismatch")
		assert.Equal(t, uint16(0), tcpVal.EntryType, "TCP entry type mismatch")
		assert.Equal(t, uint16(0), udpVal.EntryType, "UDP entry type mismatch")
		assert.Equal(t, uint8(unix.IPPROTO_TCP), tcpKey.Protocol, "TCP protocol mismatch")
		assert.Equal(t, uint8(unix.IPPROTO_UDP), udpKey.Protocol, "UDP protocol mismatch")
		assert.Equal(t, htonsPort, tcpKey.Port, "TCP port mismatch")
		assert.Equal(t, htonsPort, udpKey.Port, "UDP port mismatch")

		// Close sockets
		// Wait for both TCP and UDP listen/wait to be ready
		<-tcplistenReady
		<-udpwaitReady
		time.Sleep(1 * time.Second)
		if connTCP, err := net.Dial("tcp", "127.0.0.1:2236"); err != nil {
			t.Errorf("failed to connect to TCP socket: %v", err)
		} else {
			_, _ = connTCP.Write([]byte("CLOSE\n"))
			_ = connTCP.Close()
		}
		if connUDP, err := net.Dial("udp", "127.0.0.1:2236"); err != nil {
			t.Errorf("failed to connect to UDP socket: %v", err)
		} else {
			_, _ = connUDP.Write([]byte("CLOSE\n"))
			_ = connUDP.Close()
		}
		// Check that entries are removed
		<-tcpCloseReady
		<-udpCloseReady
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
