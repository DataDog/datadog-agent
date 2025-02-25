// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"encoding/binary"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/nettest"
	"os"
	"regexp"
	"strconv"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

var networkNamespacePattern = regexp.MustCompile(`net:\[(\d+)\]`)

func htons(port uint16) uint16 {
	return (port<<8)&0xFF00 | (port>>8)&0x00FF
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
	Addr0   uint64
	Addr1   uint64
	Netns   uint32
	Port    uint16
	Padding uint16
}

type FlowPidEntry struct {
	Pid       uint32
	EntryType uint32
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

func checkFlowPidEntry(t *testing.T, testModule *testModule, key FlowPid, expectedEntry FlowPidEntry, bound chan int, next chan struct{}, closed chan struct{}, errorExpected bool) {
	boundPort := <-bound
	if key.Port == 0 && !errorExpected {
		key.Port = htons(uint16(boundPort))
	}

	// check that an entry exists for the newly bound server
	p, ok := testModule.probe.PlatformProbe.(*probe.EBPFProbe)
	if !ok {
		close(next)
		t.Skip("skipping non eBPF probe")
	}

	m, _, err := p.Manager.GetMap("flow_pid")
	if err != nil {
		close(next)
		t.Errorf("failed to get map flow_pid: %v", err)
		return
	}

	value := FlowPidEntry{}
	if !errorExpected {
		if err := m.Lookup(&key, &value); err != nil {
			t.Log("Dumping flow_pid map ...")
			it := m.Iterate()
			a := FlowPid{}
			b := FlowPidEntry{}
			for it.Next(&a, &b) {
				t.Logf(" - key %+v value %+v", a, b)
			}
			t.Logf("The test was looking for key %+v", key)

			close(next)
			t.Errorf("Failed to lookup flow_pid: %v", err)
			return
		}

		assert.Equal(t, expectedEntry.Pid, value.Pid, "wrong pid")
		assert.Equal(t, expectedEntry.EntryType, value.EntryType, "wrong entry type")
	}

	close(next)

	// wait until the socket is closed and make sure the entry is no longer present
	<-closed
	if err := m.Lookup(&key, &value); err == nil {
		t.Errorf("flow_pid entry wasn't deleted: %+v", value)
	}

	// make sure that no other entry in the map contains the EntryPid port
	it := m.Iterate()
	a := FlowPid{}
	b := FlowPidEntry{}
	for it.Next(&a, &b) {
		if a.Port == key.Port {
			t.Errorf("flow_pid entry with matching port found %+v -> %+v", a, b)
			return
		}
	}

}

func TestFlowPidBind(t *testing.T) {
	SkipIfNotAvailable(t)
	if testEnvironment == DockerEnvironment || env.IsContainerized() {
		t.Skip("Skip tests inside docker")
	}

	checkNetworkCompatibility(t)

	if out, err := loadModule("veth"); err != nil {
		t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
	}

	ruleDefs := []*rules.RuleDefinition{
		// We use this dummy DNS rule to make sure the flow <-> pid tracking probes are loaded
		{
			ID:         "test_dns",
			Expression: `dns.question.name == "testsuite"`,
		},
	}

	pid := uint32(os.Getpid())
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
		bound := make(chan int)
		next := make(chan struct{})
		closed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet4{Port: 1234, Addr: [4]byte{0, 0, 0, 0}},
			bound,
			next,
			closed,
			false,
		)
		checkFlowPidEntry(
			t,
			test,
			FlowPid{
				Netns: netns,
				Port:  htons(1234),
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint32(0), /* BIND_ENTRY */
			},
			bound,
			next,
			closed,
			false,
		)
	})

	t.Run("test_sock_ipv4_udp_bind_127.0.0.1:1235", func(t *testing.T) {
		bound := make(chan int)
		next := make(chan struct{})
		closed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet4{Port: 1235, Addr: [4]byte{127, 0, 0, 1}},
			bound,
			next,
			closed,
			false,
		)
		checkFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
				Netns: netns,
				Port:  htons(1235),
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint32(0), /* BIND_ENTRY */
			},
			bound,
			next,
			closed,
			false,
		)
	})

	t.Run("test_sock_ipv4_udp_bind_127.0.0.1:0", func(t *testing.T) {
		bound := make(chan int)
		next := make(chan struct{})
		closed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet4{Port: 0, Addr: [4]byte{127, 0, 0, 1}},
			bound,
			next,
			closed,
			false,
		)
		checkFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
				Netns: netns,
				Port:  0, // will be set later
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint32(0), /* BIND_ENTRY */
			},
			bound,
			next,
			closed,
			false,
		)
	})

	t.Run("test_sock_ipv6_udp_bind_[::]:1236", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		bound := make(chan int)
		next := make(chan struct{})
		closed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET6,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet6{Port: 1236, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
			bound,
			next,
			closed,
			false,
		)
		checkFlowPidEntry(
			t,
			test,
			FlowPid{
				Netns: netns,
				Port:  htons(1236),
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint32(0), /* BIND_ENTRY */
			},
			bound,
			next,
			closed,
			false,
		)
	})

	t.Run("test_sock_ipv6_udp_bind_[::1]:1237", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		bound := make(chan int)
		next := make(chan struct{})
		closed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET6,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet6{Port: 1237, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			bound,
			next,
			closed,
			false,
		)
		checkFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
				Netns: netns,
				Port:  htons(1237),
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint32(0), /* BIND_ENTRY */
			},
			bound,
			next,
			closed,
			false,
		)
	})

	t.Run("test_sock_ipv6_udp_bind_[::1]:0", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		bound := make(chan int)
		next := make(chan struct{})
		closed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET6,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet6{Port: 0, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			bound,
			next,
			closed,
			false,
		)
		checkFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
				Netns: netns,
				Port:  0, // will be set later
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint32(0), /* BIND_ENTRY */
			},
			bound,
			next,
			closed,
			false,
		)
	})

	t.Run("test_sock_ipv4_tcp_bind_0.0.0.0:1234", func(t *testing.T) {
		bound := make(chan int)
		next := make(chan struct{})
		closed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 1234, Addr: [4]byte{0, 0, 0, 0}},
			bound,
			next,
			closed,
			false,
		)
		checkFlowPidEntry(
			t,
			test,
			FlowPid{
				Netns: netns,
				Port:  htons(1234),
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint32(0), /* BIND_ENTRY */
			},
			bound,
			next,
			closed,
			false,
		)
	})

	t.Run("test_sock_ipv4_tcp_bind_127.0.0.1:1235", func(t *testing.T) {
		bound := make(chan int)
		next := make(chan struct{})
		closed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 1235, Addr: [4]byte{127, 0, 0, 1}},
			bound,
			next,
			closed,
			false,
		)
		checkFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
				Netns: netns,
				Port:  htons(1235),
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint32(0), /* BIND_ENTRY */
			},
			bound,
			next,
			closed,
			false,
		)
	})

	t.Run("test_sock_ipv4_tcp_bind_127.0.0.1:0", func(t *testing.T) {
		bound := make(chan int)
		next := make(chan struct{})
		closed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 0, Addr: [4]byte{127, 0, 0, 1}},
			bound,
			next,
			closed,
			false,
		)
		checkFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
				Netns: netns,
				Port:  0, // will be set later
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint32(0), /* BIND_ENTRY */
			},
			bound,
			next,
			closed,
			false,
		)
	})

	t.Run("test_sock_ipv6_tcp_bind_[::]:1236", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		bound := make(chan int)
		next := make(chan struct{})
		closed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 1236, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
			bound,
			next,
			closed,
			false,
		)
		checkFlowPidEntry(
			t,
			test,
			FlowPid{
				Netns: netns,
				Port:  htons(1236),
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint32(0), /* BIND_ENTRY */
			},
			bound,
			next,
			closed,
			false,
		)
	})

	t.Run("test_sock_ipv6_tcp_bind_[::1]:1237", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		bound := make(chan int)
		next := make(chan struct{})
		closed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 1237, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			bound,
			next,
			closed,
			false,
		)
		checkFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
				Netns: netns,
				Port:  htons(1237),
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint32(0), /* BIND_ENTRY */
			},
			bound,
			next,
			closed,
			false,
		)
	})

	t.Run("test_sock_ipv6_tcp_bind_[::1]:0", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		bound := make(chan int)
		next := make(chan struct{})
		closed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 0, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			bound,
			next,
			closed,
			false,
		)
		checkFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
				Netns: netns,
				Port:  0, // will be set later
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint32(0), /* BIND_ENTRY */
			},
			bound,
			next,
			closed,
			false,
		)
	})
}

func TestFlowPidBindLeak(t *testing.T) {
	SkipIfNotAvailable(t)
	if testEnvironment == DockerEnvironment || env.IsContainerized() {
		t.Skip("Skip tests inside docker")
	}

	checkNetworkCompatibility(t)

	if out, err := loadModule("veth"); err != nil {
		t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
	}

	ruleDefs := []*rules.RuleDefinition{
		// We use this dummy DNS rule to make sure the flow <-> pid tracking probes are loaded
		{
			ID:         "test_dns",
			Expression: `dns.question.name == "testsuite"`,
		},
	}

	pid := uint32(os.Getpid())
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
		bound := make(chan int)
		next := make(chan struct{})
		closed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet4{Port: 2234, Addr: [4]byte{99, 99, 99, 99}},
			bound,
			next,
			closed,
			true,
		)
		checkFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 99, 99, 99, 99}),
				Netns: netns,
				Port:  htons(2234),
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint32(0), /* BIND_ENTRY */
			},
			bound,
			next,
			closed,
			true,
		)
	})

	t.Run("test_sock_ipv4_tcp_bind_99.99.99.99:2235", func(t *testing.T) {
		bound := make(chan int)
		next := make(chan struct{})
		closed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 2235, Addr: [4]byte{99, 99, 99, 99}},
			bound,
			next,
			closed,
			true,
		)
		checkFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 99, 99, 99, 99}),
				Netns: netns,
				Port:  htons(2235),
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint32(0), /* BIND_ENTRY */
			},
			bound,
			next,
			closed,
			true,
		)
	})

	t.Run("test_sock_ipv6_udp_bind_[99*]:2236", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		bound := make(chan int)
		next := make(chan struct{})
		closed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET6,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet6{Port: 2236, Addr: [16]byte{99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99}},
			bound,
			next,
			closed,
			true,
		)
		checkFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr0: binary.BigEndian.Uint64([]byte{99, 99, 99, 99, 99, 99, 99, 99}),
				Addr1: binary.BigEndian.Uint64([]byte{99, 99, 99, 99, 99, 99, 99, 99}),
				Netns: netns,
				Port:  htons(2236),
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint32(0), /* BIND_ENTRY */
			},
			bound,
			next,
			closed,
			true,
		)
	})

	t.Run("test_sock_ipv6_tcp_bind_[99*]:2237", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		bound := make(chan int)
		next := make(chan struct{})
		closed := make(chan struct{})

		go createSocketAndBind(
			t,
			syscall.AF_INET6,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet6{Port: 2237, Addr: [16]byte{99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99}},
			bound,
			next,
			closed,
			true,
		)
		checkFlowPidEntry(
			t,
			test,
			FlowPid{
				Addr0: binary.BigEndian.Uint64([]byte{99, 99, 99, 99, 99, 99, 99, 99}),
				Addr1: binary.BigEndian.Uint64([]byte{99, 99, 99, 99, 99, 99, 99, 99}),
				Netns: netns,
				Port:  htons(2237),
			},
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint32(0), /* BIND_ENTRY */
			},
			bound,
			next,
			closed,
			true,
		)
	})
}
