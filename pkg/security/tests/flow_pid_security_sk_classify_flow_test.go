// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"encoding/binary"
	"errors"
	"fmt"
	"syscall"
	"testing"

	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/nettest"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

func udpCreateSocketAndSend(sockDomain int, serverSockAddr syscall.Sockaddr, bindSockAddr syscall.Sockaddr, boundPort chan int, next chan struct{}, closed chan struct{}, errorExpected bool, clientErr chan error) {
	fd, err := syscall.Socket(sockDomain, syscall.SOCK_DGRAM, 0)
	if err != nil {
		close(boundPort)
		close(closed)
		clientErr <- fmt.Errorf("socket error: %v", err)
		clientErr <- nil
		return
	}
	defer func() {
		_ = syscall.Close(fd)
		close(closed)
		clientErr <- nil
	}()

	if bindSockAddr != nil {
		// Bind the socket to the provided address
		err = syscall.Bind(fd, bindSockAddr)
		if err != nil {
			close(boundPort)
			clientErr <- fmt.Errorf("error binding socket: %v", err)
			return
		}
	}

	// Message to send
	message := []byte("Hello, UDP receiver!")

	if err = syscall.Sendto(fd, message, 0, serverSockAddr); err != nil {
		if !errorExpected {
			close(boundPort)
			clientErr <- fmt.Errorf("sendTo error: %v", err)
			return
		}
	}

	sa, err := syscall.Getsockname(fd)
	if err != nil {
		close(boundPort)
		clientErr <- fmt.Errorf("getsockname error: %v", err)
		return
	}
	switch addr := sa.(type) {
	case *syscall.SockaddrInet6:
		boundPort <- addr.Port
	case *syscall.SockaddrInet4:
		boundPort <- addr.Port
	default:
		close(boundPort)
		clientErr <- fmt.Errorf("getsockname error: unknown Sockaddr type")
		return
	}
	<-next
}

func tcpCreateSocketAndSend(sockDomain int, serverSockAddr syscall.Sockaddr, bindSockAddr syscall.Sockaddr, boundPort chan int, connected chan struct{}, closeClientSocket chan struct{}, clientSocketClosed chan struct{}, errorExpected bool, clientErr chan error, shouldSendTCPReset bool, shouldWaitForTCPReset bool, sendTCPReset chan struct{}, tcpResetSent chan struct{}) {
	fd, err := syscall.Socket(sockDomain, syscall.SOCK_STREAM, 0)
	if err != nil {
		close(connected)
		close(boundPort)
		close(clientSocketClosed)
		clientErr <- fmt.Errorf("socket error: %v", err)
		clientErr <- nil
		return
	}
	defer func() {
		err = syscall.Close(fd)
		if err != nil {
			clientErr <- fmt.Errorf("error closing client socket: %v", err)
		}
		if shouldSendTCPReset {
			close(tcpResetSent)
		}
		close(clientSocketClosed)
		clientErr <- nil
	}()

	if bindSockAddr != nil {
		// Bind the socket to the provided address
		err = syscall.Bind(fd, bindSockAddr)
		if err != nil {
			close(connected)
			close(boundPort)
			clientErr <- fmt.Errorf("error binding socket: %v", err)
			return
		}
	}

	// Set socket option SO_REUSEADDR to reuse the address
	err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	if err != nil {
		close(connected)
		close(boundPort)
		clientErr <- fmt.Errorf("error setting client socket option: %v", err)
		return
	}

	if err = syscall.Connect(fd, serverSockAddr); err != nil {
		if !errorExpected {
			close(connected)
			close(boundPort)
			clientErr <- fmt.Errorf("connect error: %v", err)
			return
		}
	}

	// Message to send
	message := []byte("Hello, TCP receiver!")
	_, err = syscall.Write(fd, message)
	if err != nil {
		close(boundPort)
		close(connected)
		clientErr <- fmt.Errorf("error sending message to receiver: %v", err)
		return
	}

	close(connected)

	sa, err := syscall.Getsockname(fd)
	if err != nil {
		close(boundPort)
		clientErr <- fmt.Errorf("getsockname error: %v", err)
		return
	}
	switch addr := sa.(type) {
	case *syscall.SockaddrInet6:
		boundPort <- addr.Port
	case *syscall.SockaddrInet4:
		boundPort <- addr.Port
	default:
		close(boundPort)
		clientErr <- fmt.Errorf("getsockname error: unknown Sockaddr type")
		return
	}

	if shouldSendTCPReset {
		<-sendTCPReset
		linger := &syscall.Linger{
			Onoff:  1, // Enable SO_LINGER
			Linger: 0, // Force immediate RST
		}
		if err = syscall.SetsockoptLinger(fd, syscall.SOL_SOCKET, syscall.SO_LINGER, linger); err != nil {
			clientErr <- fmt.Errorf("error setting linger: %v", err)
		}
	} else if shouldWaitForTCPReset {
		<-tcpResetSent
	}
	<-closeClientSocket
}

func createSocketAndSendData(sockDomain int, sockType int, serverSockAddr syscall.Sockaddr, bindSockAddr syscall.Sockaddr, boundPort chan int, connected chan struct{}, closeClientSocket chan struct{}, clientSocketClosed chan struct{}, errorExpected bool, clientErr chan error, shouldSendTCPReset bool, shouldWaitForTCPReset bool, sendTCPReset chan struct{}, tcpResetSent chan struct{}) {
	switch sockType {
	case syscall.SOCK_STREAM:
		tcpCreateSocketAndSend(sockDomain, serverSockAddr, bindSockAddr, boundPort, connected, closeClientSocket, clientSocketClosed, errorExpected, clientErr, shouldSendTCPReset, shouldWaitForTCPReset, sendTCPReset, tcpResetSent)
	case syscall.SOCK_DGRAM:
		udpCreateSocketAndSend(sockDomain, serverSockAddr, bindSockAddr, boundPort, closeClientSocket, clientSocketClosed, errorExpected, clientErr)
	default:
		clientErr <- fmt.Errorf("unknown socket type: %d", sockType)
		clientErr <- nil
	}
}

func startServer(sockDomain int, sockType int, sockAddr syscall.Sockaddr, serverAccepted chan struct{}, stop chan struct{}, ready chan struct{}, stopped chan struct{}, serverErr chan error, shouldSendTCPReset bool, shouldWaitForTCPReset bool, sendTCPReset chan struct{}, tcpResetSent chan struct{}) {
	var connFds []int
	// Create the socket
	fd, err := syscall.Socket(sockDomain, sockType, 0)
	if err != nil {
		close(ready)
		close(serverAccepted)
		close(stopped)
		serverErr <- fmt.Errorf("error creating socket: %v", err)
		serverErr <- nil
		return
	}
	defer func(fd int) {
		if len(connFds) != 0 {
			for _, connFd := range connFds {
				_ = syscall.Close(connFd)
			}
		}
		err := syscall.Close(fd)
		if err != nil {
			serverErr <- fmt.Errorf("error closing socket: %v", err)
		}
		close(stopped)
		serverErr <- nil
	}(fd)

	// Set socket option SO_REUSEADDR to reuse the address
	err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	if err != nil {
		close(ready)
		close(serverAccepted)
		serverErr <- fmt.Errorf("error setting socket option: %v", err)
		return
	}

	// Bind the socket to the provided address
	err = syscall.Bind(fd, sockAddr)
	if err != nil {
		close(ready)
		close(serverAccepted)
		serverErr <- fmt.Errorf("error binding socket: %v", err)
		return
	}

	// Start listening on the socket
	err = syscall.Listen(fd, 10) // Allow up to 10 pending connections
	if err != nil {
		close(ready)
		close(serverAccepted)
		serverErr <- fmt.Errorf("error listening on socket: %v", err)
		return
	}

	// Create an epoll instance
	epfd, err := syscall.EpollCreate1(0)
	if err != nil {
		close(ready)
		close(serverAccepted)
		serverErr <- fmt.Errorf("error creating epoll instance: %v", err)
		return
	}
	defer syscall.Close(epfd)

	// Register the server socket for EPOLLIN (read readiness) so we get notifications when new clients are ready to be
	// accepted
	event := syscall.EpollEvent{Events: syscall.EPOLLIN, Fd: int32(fd)}
	err = syscall.EpollCtl(epfd, syscall.EPOLL_CTL_ADD, fd, &event)
	if err != nil {
		close(ready)
		close(serverAccepted)
		serverErr <- fmt.Errorf("error adding socket to epoll: %v", err)
		return
	}

	close(ready)

	// Monitor incoming connections
	events := make([]syscall.EpollEvent, 10) // Buffer for events
	var serverAcceptedChanClosed bool
	var epollRetryCount int
	for {
		select {
		case <-stop:
			if !serverAcceptedChanClosed {
				close(serverAccepted)
			}
			return
		default:
			// Wait for events with a timeout of 1 second
			n, err := syscall.EpollWait(epfd, events, 100)
			if err != nil && !errors.Is(err, syscall.EINTR) {
				close(serverAccepted)
				serverErr <- fmt.Errorf("error in epoll wait: %v", err)
				return
			}
			if epollRetryCount > 100 {
				// prevent a deadlock in case the connection couldn't be established for some reason
				if !serverAcceptedChanClosed {
					close(serverAccepted)
				}
				serverErr <- fmt.Errorf("accept timed out after 10s")
				return
			}

			// Accept new connections if ready
			if n > 0 {
				connFd, _, err := syscall.Accept(fd)
				if err != nil {
					close(serverAccepted)
					serverErr <- fmt.Errorf("error accepting connection: %v", err)
					return
				}

				// Message to send
				message := []byte("Hello, TCP sender!")
				_, err = syscall.Write(connFd, message)
				if err != nil {
					close(serverAccepted)
					serverErr <- fmt.Errorf("error sending message to sender: %v", err)
					return
				}

				close(serverAccepted)
				serverAcceptedChanClosed = true

				if shouldWaitForTCPReset {
					<-tcpResetSent
				}
				if shouldSendTCPReset {
					<-sendTCPReset
					linger := &syscall.Linger{
						Onoff:  1, // Enable SO_LINGER
						Linger: 0, // Force immediate RST
					}
					if err = syscall.SetsockoptLinger(connFd, syscall.SOL_SOCKET, syscall.SO_LINGER, linger); err != nil {
						close(tcpResetSent)
						serverErr <- fmt.Errorf("error setting linger: %v", err)
						return
					}
					// Closing now will send RST
					if err = syscall.Close(connFd); err != nil {
						close(tcpResetSent)
						serverErr <- fmt.Errorf("error closing client connection fd: %v", err)
						return
					}
					close(tcpResetSent)
				} else {
					connFds = append(connFds, connFd)
				}
			} else {
				epollRetryCount++
			}
		}
	}
}

func getFlowPidMap(t *testing.T, testModule *testModule) *ebpf.Map {
	p, ok := testModule.probe.PlatformProbe.(*probe.EBPFProbe)
	if !ok {
		t.Errorf("probe type isn't EBPF")
		return nil
	}

	m, _, err := p.Manager.GetMap("flow_pid")
	if err != nil {
		t.Errorf("failed to get map flow_pid: %v", err)
		return nil
	}
	return m
}

func assertFlowPidEntry(t *testing.T, m *ebpf.Map, key FlowPid, expectedEntry FlowPidEntry) {
	value := FlowPidEntry{}

	// look up entry
	if err := m.Lookup(&key, &value); err != nil {
		dumpMap(t, m)
		t.Errorf("couldn't find flow_pid entry %+v: %+v", key, err)
		return
	}

	// assert entry is valid
	assert.Equal(t, expectedEntry.Pid, value.Pid, "wrong pid")
	assert.Equal(t, expectedEntry.EntryType, value.EntryType, "wrong entry type")
}

func assertEmptyFlowPid(t *testing.T, m *ebpf.Map, key FlowPid) {
	value := FlowPidEntry{}

	// look up entry
	if err := m.Lookup(&key, &value); err == nil {
		dumpMap(t, m)
		t.Errorf("flow_pid entry %+v wasn't deleted: %+v", key, value)
		return
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

	t.Run("client_sock_ipv4_udp_sendto_127.0.0.1:1123", func(t *testing.T) {

		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go createSocketAndSendData(
			syscall.AF_INET,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet4{Port: 1123, Addr: [4]byte{127, 0, 0, 1}},
			nil,
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// close the client
		close(closeClientSocket)
		<-clientSocketClosed

		// everything has been released, check that no FlowPid entry leaked on the client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
			},
		)

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv6_udp_sendto_[::1]:2123", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		if listener, err := nettest.NewLocalPacketListener("udp6"); err != nil {
			t.Skipf("couldn't create local packet listener: %v", err)
		} else {
			_ = listener.Close()
		}

		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go createSocketAndSendData(
			syscall.AF_INET6,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet6{Port: 2123, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			nil,
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// close the client
		close(closeClientSocket)
		<-clientSocketClosed

		// everything has been released, check that no FlowPid entry leaked on the client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
			},
		)

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv6_udp_sendto_127.0.0.1:3123", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		if listener, err := nettest.NewLocalPacketListener("udp6"); err != nil {
			t.Skipf("couldn't create local packet listener: %v", err)
		} else {
			_ = listener.Close()
		}

		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go createSocketAndSendData(
			syscall.AF_INET6,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet4{Port: 3123, Addr: [4]byte{127, 0, 0, 1}},
			nil,
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// close the client
		close(closeClientSocket)
		<-clientSocketClosed

		// everything has been released, check that no FlowPid entry leaked on the client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
			},
		)

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv4_udp_bind_127.0.0.1:9001_sendto_127.0.0.1:4123", func(t *testing.T) {

		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go createSocketAndSendData(
			syscall.AF_INET,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet4{Port: 4123, Addr: [4]byte{127, 0, 0, 1}},
			&syscall.SockaddrInet4{Port: 9001, Addr: [4]byte{127, 0, 0, 1}},
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		// retrieve client bound port
		boundPort := <-clientBoundPort
		assert.Equal(t, 9001, boundPort, "invalid bound port")

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9001),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// close the client
		close(closeClientSocket)
		<-clientSocketClosed

		// everything has been released, check that no FlowPid entry leaked on the client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9001),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv4_udp_bind_0.0.0.0:9002_sendto_127.0.0.1:5123", func(t *testing.T) {

		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go createSocketAndSendData(
			syscall.AF_INET,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet4{Port: 5123, Addr: [4]byte{127, 0, 0, 1}},
			&syscall.SockaddrInet4{Port: 9002, Addr: [4]byte{0, 0, 0, 0}},
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		// retrieve client bound port
		boundPort := <-clientBoundPort
		assert.Equal(t, 9002, boundPort, "invalid bound port")

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9002),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// close the client
		close(closeClientSocket)
		<-clientSocketClosed

		// everything has been released, check that no FlowPid entry leaked on the client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9002),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9002),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv6_udp_bind_[::1]:9003_sendto_[::1]:6123", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		if listener, err := nettest.NewLocalPacketListener("udp6"); err != nil {
			t.Skipf("couldn't create local packet listener: %v", err)
		} else {
			_ = listener.Close()
		}

		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go createSocketAndSendData(
			syscall.AF_INET6,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet6{Port: 6123, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			&syscall.SockaddrInet6{Port: 9003, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		// retrieve client bound port
		boundPort := <-clientBoundPort
		assert.Equal(t, 9003, boundPort, "invalid bound port")

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9003),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// close the client
		close(closeClientSocket)
		<-clientSocketClosed

		// everything has been released, check that no FlowPid entry leaked on the client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9003),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv6_udp_bind_[::]:9004_sendto_[::1]:7123", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		if listener, err := nettest.NewLocalPacketListener("udp6"); err != nil {
			t.Skipf("couldn't create local packet listener: %v", err)
		} else {
			_ = listener.Close()
		}

		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go createSocketAndSendData(
			syscall.AF_INET6,
			syscall.SOCK_DGRAM,
			&syscall.SockaddrInet6{Port: 7123, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			&syscall.SockaddrInet6{Port: 9004, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		// retrieve client bound port
		boundPort := <-clientBoundPort
		assert.Equal(t, 9004, boundPort, "invalid bound port")

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9004),
				Addr1: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// close the client
		close(closeClientSocket)
		<-clientSocketClosed

		// everything has been released, check that no FlowPid entry leaked on the client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9004),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9004),
				Addr1: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv4_tcp_connect_127.0.0.1:1234_server_sock_ipv4_tcp_listen_127.0.0.1:1234_client_reset", func(t *testing.T) {
		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 1234, Addr: [4]byte{127, 0, 0, 1}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			false,
			true,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 1234, Addr: [4]byte{127, 0, 0, 1}},
			nil,
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			true,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(1234),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)

		// now that the client and server are set up, trigger reset on the client side
		close(sendTCPReset)
		// in order to send TCP RST from the client side, we also need to close the client socket
		close(closeClientSocket)
		// wait until the TCP reset packet is sent
		<-tcpResetSent
		// make sure the client socket is closed
		<-closeClientSocket

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed

		// everything has been released, check that no FlowPid entry leaked
		// client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)
		// server side
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(1234),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv4_tcp_connect_127.0.0.1:2234_server_sock_ipv4_tcp_listen_127.0.0.1:2234_server_reset", func(t *testing.T) {
		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 2234, Addr: [4]byte{127, 0, 0, 1}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			true,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 2234, Addr: [4]byte{127, 0, 0, 1}},
			nil,
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			true,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(2234),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)

		// now that the client and server are set up, trigger reset on the server side
		close(sendTCPReset)
		// wait until the TCP reset packet is sent
		<-tcpResetSent

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed
		// and close the client as well
		close(closeClientSocket)
		<-clientSocketClosed

		// everything has been released, check that no FlowPid entry leaked
		// client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)
		// server side
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(2234),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv4_tcp_connect_127.0.0.1:3234_server_sock_ipv4_tcp_listen_127.0.0.1:3234_client_fin", func(t *testing.T) {
		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 3234, Addr: [4]byte{127, 0, 0, 1}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 3234, Addr: [4]byte{127, 0, 0, 1}},
			nil,
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(3234),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)

		// we can now close the client
		close(closeClientSocket)
		<-clientSocketClosed

		// everything has been released, check that no FlowPid entry leaked
		// client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed

		// server side
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(3234),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv4_tcp_connect_127.0.0.1:4234_server_sock_ipv4_tcp_listen_127.0.0.1:4234_server_fin", func(t *testing.T) {
		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 4234, Addr: [4]byte{127, 0, 0, 1}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 4234, Addr: [4]byte{127, 0, 0, 1}},
			nil,
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(4234),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed

		// server side
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(4234),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)

		// we can now close the client
		close(closeClientSocket)
		<-clientSocketClosed

		// everything has been released, check that no FlowPid entry leaked
		// client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv6_tcp_connect_[::1]:5234_server_sock_ipv6_tcp_listen_[::1]:5234_client_fin", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		if listener, err := nettest.NewLocalPacketListener("udp6"); err != nil {
			t.Skipf("couldn't create local packet listener: %v", err)
		} else {
			_ = listener.Close()
		}

		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 5234, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 5234, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			nil,
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(5234),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)

		// we can now close the client
		close(closeClientSocket)
		<-clientSocketClosed

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed

		// check that no FlowPid entry leaked from the client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// server side
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(5234),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv6_tcp_connect_[::1]:6234_server_sock_ipv6_tcp_listen_[::1]:6234_server_fin", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		if listener, err := nettest.NewLocalPacketListener("udp6"); err != nil {
			t.Skipf("couldn't create local packet listener: %v", err)
		} else {
			_ = listener.Close()
		}

		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 6234, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 6234, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			nil,
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(6234),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed

		// check that no FlowPid entry leaked from the server side
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(6234),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// we can now close the client
		close(closeClientSocket)
		<-clientSocketClosed

		// client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv6_tcp_connect_[::1]:7234_server_sock_ipv6_tcp_listen_[::1]:7234_client_reset", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		if listener, err := nettest.NewLocalPacketListener("udp6"); err != nil {
			t.Skipf("couldn't create local packet listener: %v", err)
		} else {
			_ = listener.Close()
		}

		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 7234, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			false,
			true,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 7234, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			nil,
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			true,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(7234),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)
		// now that the client and server are set up, trigger reset on the client side
		close(sendTCPReset)
		// in order to send TCP RST from the client side, we also need to close the client socket
		close(closeClientSocket)
		// wait until the TCP reset packet is sent
		<-tcpResetSent
		// make sure the client socket is closed
		<-closeClientSocket

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed
		// and close the client as well

		// check that no FlowPid entry leaked from the client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// server side
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(7234),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv6_tcp_connect_[::1]:8234_server_sock_ipv6_tcp_listen_[::1]:8234_server_reset", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		if listener, err := nettest.NewLocalPacketListener("udp6"); err != nil {
			t.Skipf("couldn't create local packet listener: %v", err)
		} else {
			_ = listener.Close()
		}

		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 8234, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			true,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 8234, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			nil,
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			true,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(8234),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)

		// now that the client and server are set up, trigger reset on the server side
		close(sendTCPReset)
		// wait until the TCP reset packet is sent
		<-tcpResetSent

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed

		// and close the client as well
		close(closeClientSocket)
		<-clientSocketClosed

		// check that no FlowPid entry leaked from the server side
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(8234),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv4_tcp_connect_127.0.0.1:9234_server_sock_ipv4_tcp_listen_0.0.0.0:9234_client_reset", func(t *testing.T) {
		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 9234, Addr: [4]byte{0, 0, 0, 0}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			false,
			true,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 9234, Addr: [4]byte{127, 0, 0, 1}},
			nil,
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			true,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9234),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)

		// now that the client and server are set up, trigger reset on the client side
		close(sendTCPReset)
		// in order to send TCP RST from the client side, we also need to close the client socket
		close(closeClientSocket)
		// wait until the TCP reset packet is sent
		<-tcpResetSent
		// make sure the client socket is closed
		<-closeClientSocket

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed
		// and close the client as well

		// everything has been released, check that no FlowPid entry leaked
		// client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)
		// server side
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9234),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9234),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv4_tcp_connect_127.0.0.1:1334_server_sock_ipv4_tcp_listen_0.0.0.0:1334_server_reset", func(t *testing.T) {
		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 1334, Addr: [4]byte{0, 0, 0, 0}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			true,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 1334, Addr: [4]byte{127, 0, 0, 1}},
			nil,
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			true,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(1334),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)

		// now that the client and server are set up, trigger reset on the server side
		close(sendTCPReset)
		// wait until the TCP reset packet is sent
		<-tcpResetSent

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed

		// and close the client as well
		close(closeClientSocket)
		<-clientSocketClosed

		// everything has been released, check that no FlowPid entry leaked
		// client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)
		// server side
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(2234),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(2234),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv4_tcp_connect_127.0.0.1:2334_server_sock_ipv4_tcp_listen_0.0.0.0:2334_client_fin", func(t *testing.T) {
		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 2334, Addr: [4]byte{0, 0, 0, 0}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 2334, Addr: [4]byte{127, 0, 0, 1}},
			nil,
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(2334),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)

		// we can now close the client
		close(closeClientSocket)
		<-clientSocketClosed

		// everything has been released, check that no FlowPid entry leaked
		// client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed

		// server side
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(2334),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(2334),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv4_tcp_connect_127.0.0.1:3334_server_sock_ipv4_tcp_listen_0.0.0.0:3334_server_fin", func(t *testing.T) {
		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 3334, Addr: [4]byte{0, 0, 0, 0}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 3334, Addr: [4]byte{127, 0, 0, 1}},
			nil,
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(3334),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed

		// server side
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(3334),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(3334),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// we can now close the client
		close(closeClientSocket)
		<-clientSocketClosed

		// everything has been released, check that no FlowPid entry leaked
		// client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv6_tcp_connect_[::1]:4334_server_sock_ipv6_tcp_listen_[::]:4334_client_fin", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		if listener, err := nettest.NewLocalPacketListener("udp6"); err != nil {
			t.Skipf("couldn't create local packet listener: %v", err)
		} else {
			_ = listener.Close()
		}

		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 4334, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 4334, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			nil,
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(4334),
				Addr1: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(4334),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* BIND_ENTRY */
			},
		)

		// we can now close the client
		close(closeClientSocket)
		<-clientSocketClosed

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed

		// check that no FlowPid entry leaked from the client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// server side
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(4334),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(4334),
				Addr1: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv6_tcp_connect_[::1]:5334_server_sock_ipv6_tcp_listen_[::]:5334_server_fin", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		if listener, err := nettest.NewLocalPacketListener("udp6"); err != nil {
			t.Skipf("couldn't create local packet listener: %v", err)
		} else {
			_ = listener.Close()
		}

		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 5334, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 5334, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			nil,
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(5334),
				Addr1: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(5334),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed

		// check that no FlowPid entry leaked from the server side
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(5334),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(5334),
				Addr1: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// we can now close the client
		close(closeClientSocket)
		<-clientSocketClosed

		// client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv6_tcp_connect_[::1]:6334_server_sock_ipv6_tcp_listen_[::]:6334_client_reset", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		if listener, err := nettest.NewLocalPacketListener("udp6"); err != nil {
			t.Skipf("couldn't create local packet listener: %v", err)
		} else {
			_ = listener.Close()
		}

		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 6334, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			false,
			true,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 6334, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			nil,
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			true,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(6334),
				Addr1: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(6334),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* BIND_ENTRY */
			},
		)

		// now that the client and server are set up, trigger reset on the client side
		close(sendTCPReset)
		// in order to send TCP RST from the client side, we also need to close the client socket
		close(closeClientSocket)
		// wait until the TCP reset packet is sent
		<-tcpResetSent
		// make sure the client socket is closed
		<-closeClientSocket

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed
		// and close the client as well

		// check that no FlowPid entry leaked from the client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// server side
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(6334),
				Addr1: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(6334),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv6_tcp_connect_[::1]:7334_server_sock_ipv6_tcp_listen_[::]:7334_server_reset", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		if listener, err := nettest.NewLocalPacketListener("udp6"); err != nil {
			t.Skipf("couldn't create local packet listener: %v", err)
		} else {
			_ = listener.Close()
		}

		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 7334, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			true,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 7334, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			nil,
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			true,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(7334),
				Addr1: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(7334),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// now that the client and server are set up, trigger reset on the server side
		close(sendTCPReset)
		// wait until the TCP reset packet is sent
		<-tcpResetSent

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed

		// and close the client as well
		close(closeClientSocket)
		<-clientSocketClosed

		// check that no FlowPid entry leaked from the server side
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(7334),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(7334),
				Addr1: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv4_tcp_bind_127.0.0.1:9005_connect_127.0.0.1:8334_server_sock_ipv4_tcp_listen_127.0.0.1:8334_client_fin", func(t *testing.T) {
		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 8334, Addr: [4]byte{127, 0, 0, 1}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 8334, Addr: [4]byte{127, 0, 0, 1}},
			&syscall.SockaddrInet4{Port: 9005, Addr: [4]byte{127, 0, 0, 1}},
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := <-clientBoundPort
		assert.Equal(t, 9005, clientPort, "wrong bound port")

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9005),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(8334),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)

		// we can now close the server
		close(closeClientSocket)
		<-clientSocketClosed

		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9005),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed

		// server side
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(8334),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv4_tcp_bind_0.0.0.0:9006_connect_127.0.0.1:9334_server_sock_ipv4_tcp_listen_127.0.0.1:9334_client_fin", func(t *testing.T) {
		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 9334, Addr: [4]byte{127, 0, 0, 1}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 9334, Addr: [4]byte{127, 0, 0, 1}},
			&syscall.SockaddrInet4{Port: 9006, Addr: [4]byte{0, 0, 0, 0}},
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := <-clientBoundPort
		assert.Equal(t, 9006, clientPort, "wrong bound port")

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9006),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9006),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9334),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)

		// we can now close the server
		close(closeClientSocket)
		<-clientSocketClosed

		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9006),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed

		// server side
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9334),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv6_tcp_bind_[::1]:9007_connect_[::1]:1434_server_sock_ipv6_tcp_listen_[::1]:1434_client_fin", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		if listener, err := nettest.NewLocalPacketListener("udp6"); err != nil {
			t.Skipf("couldn't create local packet listener: %v", err)
		} else {
			_ = listener.Close()
		}

		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 1434, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 1434, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			&syscall.SockaddrInet6{Port: 9007, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := <-clientBoundPort
		assert.Equal(t, 9007, clientPort, "wrong bound port")

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9007),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(1434),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)

		// we can now close the client
		close(closeClientSocket)
		<-clientSocketClosed

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed

		// check that no FlowPid entry leaked from the client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9007),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// server side
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(1434),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv6_tcp_bind_[::]:9008_connect_[::1]:2434_server_sock_ipv6_tcp_listen_[::1]:2434_client_fin", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		if listener, err := nettest.NewLocalPacketListener("udp6"); err != nil {
			t.Skipf("couldn't create local packet listener: %v", err)
		} else {
			_ = listener.Close()
		}

		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 2434, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 2434, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
			&syscall.SockaddrInet6{Port: 9008, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := <-clientBoundPort
		assert.Equal(t, 9008, clientPort, "wrong bound port")

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9008),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9008),
				Addr1: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(2434),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)

		// we can now close the client
		close(closeClientSocket)
		<-clientSocketClosed

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed

		// check that no FlowPid entry leaked from the client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9008),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(9008),
				Addr1: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// server side
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(2434),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv4_tcp_connect_127.0.0.1:3434_server_sock_ipv6_tcp_listen_[::]:3434_client_fin", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		if listener, err := nettest.NewLocalPacketListener("udp6"); err != nil {
			t.Skipf("couldn't create local packet listener: %v", err)
		} else {
			_ = listener.Close()
		}

		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 3434, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 3434, Addr: [4]byte{127, 0, 0, 1}},
			nil,
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 127, 255, 255, 0, 0}),
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(3434),
				Addr1: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  htons(3434),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  htons(3434),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 127, 255, 255, 0, 0}),
			},
		)

		// we can now close the client
		close(closeClientSocket)
		<-clientSocketClosed

		// everything has been released, check that no FlowPid entry leaked
		// client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 127, 255, 255, 0, 0}),
			},
		)

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed

		// server side
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(3434),
				Addr1: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  htons(3434),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  htons(3434),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 127, 255, 255, 0, 0}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv6_tcp_connect_[::ffff:127.0.0.1]:4434_server_sock_ipv6_tcp_listen_[::]:4434_client_fin", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		if listener, err := nettest.NewLocalPacketListener("udp6"); err != nil {
			t.Skipf("couldn't create local packet listener: %v", err)
		} else {
			_ = listener.Close()
		}

		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 4434, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 4434, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 127, 0, 0, 1}},
			nil,
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 127, 255, 255, 0, 0}),
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(4434),
				Addr1: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  htons(4434),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  htons(4434),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 127, 255, 255, 0, 0}),
			},
		)

		// we can now close the client
		close(closeClientSocket)
		<-clientSocketClosed

		// everything has been released, check that no FlowPid entry leaked
		// client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 127, 255, 255, 0, 0}),
			},
		)

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed

		// server side
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(4434),
				Addr1: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  htons(4434),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  htons(4434),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 127, 255, 255, 0, 0}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv6_tcp_connect_[::ffff:127.0.0.1]:5434_server_sock_ipv4_tcp_listen_0.0.0.0:5434_client_fin", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		if listener, err := nettest.NewLocalPacketListener("udp6"); err != nil {
			t.Skipf("couldn't create local packet listener: %v", err)
		} else {
			_ = listener.Close()
		}

		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 5434, Addr: [4]byte{0, 0, 0, 0}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 5434, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 127, 0, 0, 1}},
			nil,
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 127, 255, 255, 0, 0}),
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(5434),
				Addr1: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  htons(5434),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  htons(5434),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 127, 255, 255, 0, 0}),
			},
		)

		// we can now close the client
		close(closeClientSocket)
		<-clientSocketClosed

		// everything has been released, check that no FlowPid entry leaked
		// client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 127, 255, 255, 0, 0}),
			},
		)

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed

		// server side
		assertEmptyFlowPid(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(5434),
				Addr1: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  htons(5434),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  htons(5434),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 127, 255, 255, 0, 0}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})

	t.Run("client_sock_ipv6_tcp_connect_[::ffff:127.0.0.1]:6434_server_sock_ipv4_tcp_listen_127.0.0.1:6434_client_fin", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		if listener, err := nettest.NewLocalPacketListener("udp6"); err != nil {
			t.Skipf("couldn't create local packet listener: %v", err)
		} else {
			_ = listener.Close()
		}

		clientBoundPort := make(chan int)
		closeClientSocket := make(chan struct{})
		clientSocketClosed := make(chan struct{})
		clientConnected := make(chan struct{})
		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		clientErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 6434, Addr: [4]byte{127, 0, 0, 1}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 6434, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 127, 0, 0, 1}},
			nil,
			clientBoundPort,
			clientConnected,
			closeClientSocket,
			clientSocketClosed,
			false,
			clientErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		<-clientConnected
		<-serverAccepted

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 127, 255, 255, 0, 0}),
			},
		)

		// check server flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(6434),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  htons(6434),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 127, 255, 255, 0, 0}),
			},
		)

		// we can now close the client
		close(closeClientSocket)
		<-clientSocketClosed

		// everything has been released, check that no FlowPid entry leaked
		// client side
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  clientPort,
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 127, 255, 255, 0, 0}),
			},
		)

		// we can now close the server
		close(closeServerSocket)
		<-serverSocketClosed

		// server side
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  htons(6434),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  htons(6434),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 127, 255, 255, 0, 0}),
			},
		)

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
	})
}

func TestFlowPidSecuritySKClassifyFlowLeaks(t *testing.T) {
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

	t.Run("client_sock_ipv4_udp_sendto_127.0.0.1:1111_sendto_127.0.0.1:1112", func(t *testing.T) {
		var port1, port2 uint16

		fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
		if err != nil {
			t.Errorf("socket error: %v", err)
			return
		}
		defer func() {
			_ = syscall.Close(fd)
		}()

		// Message to send
		message := []byte("Hello, UDP receiver!")

		if err = syscall.Sendto(fd, message, 0, &syscall.SockaddrInet4{Port: 1111, Addr: [4]byte{127, 0, 0, 1}}); err != nil {
			t.Errorf("sendTo error: %v", err)
			return
		}

		sa, err := syscall.Getsockname(fd)
		if err != nil {
			t.Errorf("getsockname error: %v", err)
			return
		}
		switch addr := sa.(type) {
		case *syscall.SockaddrInet4:
			port1 = htons(uint16(addr.Port))
		default:
			t.Errorf("getsockname error: unknown Sockaddr type")
			return
		}

		if err = syscall.Sendto(fd, message, 0, &syscall.SockaddrInet4{Port: 1112, Addr: [4]byte{127, 0, 0, 1}}); err != nil {
			t.Errorf("sendTo error: %v", err)
			return
		}

		sa, err = syscall.Getsockname(fd)
		if err != nil {
			t.Errorf("getsockname error: %v", err)
			return
		}
		switch addr := sa.(type) {
		case *syscall.SockaddrInet4:
			port2 = htons(uint16(addr.Port))
		default:
			t.Errorf("getsockname error: unknown Sockaddr type")
			return
		}

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  port2,
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)

		if port1 != port2 {
			assertEmptyFlowPid(
				t,
				m,
				// client entry key
				FlowPid{
					Netns: netns,
					Port:  port1,
				},
			)
		}

		_ = syscall.Close(fd)

		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  port2,
			},
		)
	})

	t.Run("client_sock_ipv6_udp_bind_[::1]:1113_sendto_127.0.0.1:1114", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		if listener, err := nettest.NewLocalPacketListener("udp6"); err != nil {
			t.Skipf("couldn't create local packet listener: %v", err)
		} else {
			_ = listener.Close()
		}

		fd, err := syscall.Socket(syscall.AF_INET6, syscall.SOCK_DGRAM, 0)
		if err != nil {
			t.Errorf("socket error: %v", err)
			return
		}
		defer func() {
			_ = syscall.Close(fd)
		}()

		// Bind the socket to the provided address
		err = syscall.Bind(fd, &syscall.SockaddrInet6{Port: 1113, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}})
		if err != nil {
			t.Errorf("error binding socket: %v", err)
			return
		}

		// Message to send
		message := []byte("Hello, UDP receiver!")

		if err = syscall.Sendto(fd, message, 0, &syscall.SockaddrInet4{Port: 1114, Addr: [4]byte{127, 0, 0, 1}}); err == nil {
			t.Errorf("expected to fail sending to 127.0.0.1:1114")
			return
		}

		m := getFlowPidMap(t, test)
		if m == nil {
			t.Fatalf("failed to get map flow_pid")
			return
		}

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(1113),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(1114),
				Addr0: binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 1, 0, 0, 127}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			FlowPid{
				Netns: netns,
				Port:  htons(1114),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 127, 255, 255, 0, 0}),
			},
		)

		// close the client
		_ = syscall.Close(fd)

		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(1113),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)
	})

	t.Run("client_sock_ipv6_tcp_connect_127.0.0.1:1115_connect_[::1]:1116_server_sock_ipv6_tcp_listen_[::]:1116", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
		}

		if listener, err := nettest.NewLocalPacketListener("udp6"); err != nil {
			t.Skipf("couldn't create local packet listener: %v", err)
		} else {
			_ = listener.Close()
		}

		closeServerSocket := make(chan struct{})
		serverReady := make(chan struct{})
		serverAccepted := make(chan struct{})
		serverSocketClosed := make(chan struct{})
		serverErr := make(chan error, 100)
		tcpResetSent := make(chan struct{})
		sendTCPReset := make(chan struct{})

		go startServer(
			syscall.AF_INET6,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet6{Port: 1116, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
			serverAccepted,
			closeServerSocket,
			serverReady,
			serverSocketClosed,
			serverErr,
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		fd, err := syscall.Socket(syscall.AF_INET6, syscall.SOCK_STREAM, 0)
		if err != nil {
			close(closeServerSocket)
			t.Errorf("socket error: %v", err)
			return
		}
		defer func() {
			_ = syscall.Close(fd)
		}()

		if err = syscall.Connect(fd, &syscall.SockaddrInet4{Port: 1115, Addr: [4]byte{127, 0, 0, 1}}); err == nil {
			close(closeServerSocket)
			t.Errorf("expected to fail connect to 127.0.0.1:1115")
			return
		}

		var boundPort1 int
		sa, err := syscall.Getsockname(fd)
		if err != nil {
			close(closeServerSocket)
			t.Errorf("getsockname error: %v", err)
			return
		}
		switch addr := sa.(type) {
		case *syscall.SockaddrInet6:
			boundPort1 = addr.Port
		case *syscall.SockaddrInet4:
			boundPort1 = addr.Port
		default:
			close(closeServerSocket)
			t.Errorf("getsockname error: unknown Sockaddr type")
			return
		}

		m := getFlowPidMap(t, test)
		if m == nil {
			close(closeServerSocket)
			t.Fatalf("failed to get map flow_pid")
			return
		}

		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(uint16(boundPort1)),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// connect to the server properly
		if err = syscall.Connect(fd, &syscall.SockaddrInet6{Port: 1116, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}}); err != nil {
			close(closeServerSocket)
			t.Errorf("didn't expect an error connecting to 127.0.0.1:1116: %v", err)
			return
		}

		<-serverAccepted

		sa, err = syscall.Getsockname(fd)
		if err != nil {
			close(closeServerSocket)
			t.Errorf("getsockname error: %v", err)
			return
		}
		var boundPort2 int
		switch addr := sa.(type) {
		case *syscall.SockaddrInet6:
			boundPort2 = addr.Port
		case *syscall.SockaddrInet4:
			boundPort2 = addr.Port
		default:
			close(closeServerSocket)
			t.Errorf("getsockname error: unknown Sockaddr type")
			return
		}

		// check client flow_pid entry
		assertFlowPidEntry(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(uint16(boundPort2)),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// client expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(2), /* FLOW_CLASSIFICATION_ENTRY */
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(uint16(boundPort1)),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		// close the client
		_ = syscall.Close(fd)

		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(uint16(boundPort1)),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)
		assertEmptyFlowPid(
			t,
			m,
			// client entry key
			FlowPid{
				Netns: netns,
				Port:  htons(uint16(boundPort2)),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		close(closeServerSocket)
	})
}
