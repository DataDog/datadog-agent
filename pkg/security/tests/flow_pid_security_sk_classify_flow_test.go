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
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/nettest"
	"syscall"
	"testing"
)

func udpCreateSocketAndSend(sockDomain int, sockAddr syscall.Sockaddr, boundPort chan int, next chan struct{}, closed chan struct{}, errorExpected bool, clientErr chan error) {
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

	// Message to send
	message := []byte("Hello, UDP receiver!")

	if err = syscall.Sendto(fd, message, 0, sockAddr); err != nil {
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

func tcpCreateSocketAndSend(sockDomain int, sockAddr syscall.Sockaddr, boundPort chan int, connected chan struct{}, closeClientSocket chan struct{}, clientSocketClosed chan struct{}, errorExpected bool, clientErr chan error, shouldSendTCPReset bool, shouldWaitForTCPReset bool, sendTCPReset chan struct{}, tcpResetSent chan struct{}) {
	fd, err := syscall.Socket(sockDomain, syscall.SOCK_STREAM, 0)
	if err != nil {
		close(boundPort)
		close(clientSocketClosed)
		clientErr <- fmt.Errorf("socket error: %v", err)
		clientErr <- nil
		return
	}
	defer func() {
		fmt.Println("[client] stopping ...")
		err = syscall.Close(fd)
		if err != nil {
			clientErr <- fmt.Errorf("error closing client socket: %v", err)
		}
		fmt.Println("[client] socket closed")
		if shouldSendTCPReset {
			fmt.Println("[client] TCP reset sent")
			close(tcpResetSent)
		}
		close(clientSocketClosed)
		clientErr <- nil
	}()

	// Set socket option SO_REUSEADDR to reuse the address
	err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	if err != nil {
		close(boundPort)
		clientErr <- fmt.Errorf("error setting client socket option: %v", err)
		return
	}

	fmt.Println("[client] sending connect")
	if err = syscall.Connect(fd, sockAddr); err != nil {
		if !errorExpected {
			close(boundPort)
			clientErr <- fmt.Errorf("connect error: %v", err)
			return
		}
	}
	fmt.Println("[client] connect returned")
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
		fmt.Println("[client] waiting for TCPReset request")
		<-sendTCPReset
		linger := &syscall.Linger{
			Onoff:  1, // Enable SO_LINGER
			Linger: 0, // Force immediate RST
		}
		if err = syscall.SetsockoptLinger(fd, syscall.SOL_SOCKET, syscall.SO_LINGER, linger); err != nil {
			clientErr <- fmt.Errorf("error setting linger: %v", err)
		}
		fmt.Println("[client] socket set up for TCPReset, waiting for client socket closure")
	} else if shouldWaitForTCPReset {
		fmt.Println("[client] waiting for TCP Reset sent")
		<-tcpResetSent
		fmt.Println("[client] done waiting TCP Reset sent")
	}
	fmt.Println("[client] waiting for closeClientSocket")
	<-closeClientSocket
}

func createSocketAndSendData(sockDomain int, sockType int, sockAddr syscall.Sockaddr, boundPort chan int, connected chan struct{}, closeClientSocket chan struct{}, clientSocketClosed chan struct{}, errorExpected bool, clientErr chan error, shouldSendTCPReset bool, shouldWaitForTCPReset bool, sendTCPReset chan struct{}, tcpResetSent chan struct{}) {
	switch sockType {
	case syscall.SOCK_STREAM:
		tcpCreateSocketAndSend(sockDomain, sockAddr, boundPort, connected, closeClientSocket, clientSocketClosed, errorExpected, clientErr, shouldSendTCPReset, shouldWaitForTCPReset, sendTCPReset, tcpResetSent)
	case syscall.SOCK_DGRAM:
		udpCreateSocketAndSend(sockDomain, sockAddr, boundPort, closeClientSocket, clientSocketClosed, errorExpected, clientErr)
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
		close(stopped)
		serverErr <- fmt.Errorf("error creating socket: %v", err)
		serverErr <- nil
		return
	}
	defer func(fd int) {
		if len(connFds) != 0 {
			fmt.Println("[server] closing connFds")
			for _, connFd := range connFds {
				err := syscall.Close(connFd)
				fmt.Printf("[server] closing connFd %d, err: %v\n", connFd, err)
			}
		} else {
			fmt.Println("[server] no client connFD to close")
		}
		fmt.Println("[server] closing server FD")
		err := syscall.Close(fd)
		if err != nil {
			serverErr <- fmt.Errorf("error closing socket: %v", err)
		}
		fmt.Println("[server] server FD closed")
		close(stopped)
		serverErr <- nil
	}(fd)

	// Set socket option SO_REUSEADDR to reuse the address
	err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	if err != nil {
		close(ready)
		serverErr <- fmt.Errorf("error setting socket option: %v", err)
		return
	}

	// Bind the socket to address
	err = syscall.Bind(fd, sockAddr)
	if err != nil {
		close(ready)
		serverErr <- fmt.Errorf("error binding socket: %v", err)
		return
	}

	// Start listening on the socket
	err = syscall.Listen(fd, 10) // Allow up to 10 pending connections
	if err != nil {
		close(ready)
		serverErr <- fmt.Errorf("error listening on socket: %v", err)
		return
	}

	// Create an epoll instance
	epfd, err := syscall.EpollCreate1(0)
	if err != nil {
		close(ready)
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
		serverErr <- fmt.Errorf("error adding socket to epoll: %v", err)
		return
	}

	fmt.Println("[server] ready")
	close(ready)

	// Monitor incoming connections
	events := make([]syscall.EpollEvent, 10) // Buffer for events
	for {
		select {
		case <-stop:
			fmt.Println("[server] stopping")
			return
		default:
			// Wait for events with a timeout of 1 second
			n, err := syscall.EpollWait(epfd, events, 100)
			if err != nil && !errors.Is(err, syscall.EINTR) {
				serverErr <- fmt.Errorf("error in epoll wait: %v", err)
				return
			}

			// Accept new connections if ready
			if n > 0 {
				connFd, _, err := syscall.Accept(fd)
				if err != nil {
					serverErr <- fmt.Errorf("error accepting connection: %v", err)
					return
				}
				fmt.Println("[server] accepted connection")
				close(serverAccepted)

				if shouldWaitForTCPReset {
					fmt.Println("[server] waiting for tcpResetSent")
					<-tcpResetSent
					fmt.Println("[server] done waiting for tcpResetSent")
				}
				if shouldSendTCPReset {
					fmt.Println("[server] waiting for sendTCPReset")
					<-sendTCPReset
					fmt.Println("[server] done waiting for sendTCPReset")
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
					fmt.Println("[server] client connFD closed - TCP reset sent")
					close(tcpResetSent)
				} else {
					connFds = append(connFds, connFd)
				}
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

	t.Run("client_sock_ipv4_udp_sendto_127.0.0.1:1234", func(t *testing.T) {

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
			&syscall.SockaddrInet4{Port: 1234, Addr: [4]byte{127, 0, 0, 1}},
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
		fmt.Println("TEST stopping client")
		close(closeClientSocket)
		<-clientSocketClosed
		fmt.Println("TEST client stopped")

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

	t.Run("client_sock_ipv6_udp_sendto_[::1]:2234", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
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
			&syscall.SockaddrInet6{Port: 2234, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
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
		fmt.Println("TEST stopping client")
		close(closeClientSocket)
		<-clientSocketClosed
		fmt.Println("TEST client stopped")

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

	t.Run("client_sock_ipv6_udp_sendto_127.0.0.1:3234", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
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
			&syscall.SockaddrInet4{Port: 3234, Addr: [4]byte{127, 0, 0, 1}},
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
		fmt.Println("TEST stopping client")
		close(closeClientSocket)
		<-clientSocketClosed
		fmt.Println("TEST client stopped")

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

	t.Run("client_sock_ipv4_tcp_connect_127.0.0.1:1234_server_sock_ipv4_tcp_client_reset", func(t *testing.T) {
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

		fmt.Println("TEST Waiting for client to be connected and server to accept")
		<-clientConnected
		<-serverAccepted
		fmt.Println("TEST client connected and server accepted !")

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		t.Logf("TEST checking client entry")
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
		t.Logf("TEST checking server entry")
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

		fmt.Println("TEST triggering reset")
		// now that the client and server are set up, trigger reset on the client side
		close(sendTCPReset)
		// in order to send TCP RST from the client side, we also need to close the client socket
		fmt.Println("TEST stopping client")
		close(closeClientSocket)
		// wait until the TCP reset packet is sent
		<-tcpResetSent
		fmt.Println("TEST reset sent")
		// make sure the client socket is closed
		<-closeClientSocket
		fmt.Println("TEST client stopped")

		// we can now close the server
		fmt.Println("TEST stopping server")
		close(closeServerSocket)
		<-serverSocketClosed
		fmt.Println("TEST server stopped")
		// and close the client as well

		// everything has been released, check that no FlowPid entry leaked
		// client side
		fmt.Println("TEST checking client entry leak")
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
		fmt.Println("TEST checking server entry leak")
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

		fmt.Println("checking server errors")

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		fmt.Println("checking client errors")
		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
		fmt.Println("done")
	})

	t.Run("client_sock_ipv4_tcp_connect_127.0.0.1:1234_server_sock_ipv4_tcp_server_reset", func(t *testing.T) {
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
			true,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 1234, Addr: [4]byte{127, 0, 0, 1}},
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

		fmt.Println("TEST Waiting for client to be connected and server to accept")
		<-clientConnected
		<-serverAccepted
		fmt.Println("TEST client connected and server accepted !")

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		fmt.Println("TEST checking client entry")
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
		fmt.Println("TEST checking server entry")
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

		fmt.Println("TEST triggering reset")
		// now that the client and server are set up, trigger reset on the server side
		close(sendTCPReset)
		// wait until the TCP reset packet is sent
		<-tcpResetSent
		fmt.Println("TEST reset sent")

		// we can now close the server
		fmt.Println("TEST stopping server")
		close(closeServerSocket)
		<-serverSocketClosed
		fmt.Println("TEST server stopped")
		// and close the client as well
		fmt.Println("TEST stopping client")
		close(closeClientSocket)
		<-clientSocketClosed
		fmt.Println("TEST client stopped")

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

		fmt.Println("checking server errors")

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		fmt.Println("checking client errors")
		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
		fmt.Println("done")
	})

	t.Run("client_sock_ipv4_tcp_connect_127.0.0.1:2234_server_sock_ipv4_tcp_client_fin", func(t *testing.T) {
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
			false,
			false,
			sendTCPReset,
			tcpResetSent,
		)

		<-serverReady

		go createSocketAndSendData(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			&syscall.SockaddrInet4{Port: 2234, Addr: [4]byte{127, 0, 0, 1}},
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

		fmt.Println("TEST Waiting for client to be connected and server to accept")
		<-clientConnected
		<-serverAccepted
		fmt.Println("TEST client connected and server accepted !")

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		fmt.Println("TEST checking client entry")
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
		fmt.Println("TEST checking server entry")
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

		// we can now close the client
		fmt.Println("TEST stopping client")
		close(closeClientSocket)
		<-clientSocketClosed
		fmt.Println("TEST client stopped")

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
		fmt.Println("TEST stopping server")
		close(closeServerSocket)
		<-serverSocketClosed
		fmt.Println("TEST server stopped")

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

		fmt.Println("checking server errors")

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		fmt.Println("checking client errors")
		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
		fmt.Println("done")
	})

	t.Run("client_sock_ipv4_tcp_connect_127.0.0.1:3234_server_sock_ipv4_tcp_server_fin", func(t *testing.T) {
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

		fmt.Println("TEST Waiting for client to be connected and server to accept")
		<-clientConnected
		<-serverAccepted
		fmt.Println("TEST client connected and server accepted !")

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		fmt.Println("TEST checking client entry")
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
		fmt.Println("TEST checking server entry")
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

		// we can now close the server
		fmt.Println("TEST stopping server")
		close(closeServerSocket)
		<-serverSocketClosed
		fmt.Println("TEST server stopped")

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

		// we can now close the client
		fmt.Println("TEST stopping client")
		close(closeClientSocket)
		<-clientSocketClosed
		fmt.Println("TEST client stopped")

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

		fmt.Println("checking server errors")

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		fmt.Println("checking client errors")
		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
		fmt.Println("done")
	})

	t.Run("client_sock_ipv6_tcp_connect_[::1]:4234_server_sock_ipv6_tcp_client_fin", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
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
			&syscall.SockaddrInet6{Port: 4234, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
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
			&syscall.SockaddrInet6{Port: 4234, Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
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

		fmt.Println("TEST Waiting for client to be connected and server to accept")
		<-clientConnected
		<-serverAccepted
		fmt.Println("TEST client connected and server accepted !")

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		fmt.Println("TEST checking client entry")
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
		fmt.Println("TEST checking server entry")
		assertFlowPidEntry(
			t,
			m,
			// server entry key
			FlowPid{
				Netns: netns,
				Port:  htons(4234),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
			// server expected entry
			FlowPidEntry{
				Pid:       pid,
				EntryType: uint16(0), /* BIND_ENTRY */
			},
		)

		// we can now close the client
		fmt.Println("TEST stopping client")
		close(closeClientSocket)
		<-clientSocketClosed
		fmt.Println("TEST client stopped")

		// we can now close the server
		fmt.Println("TEST stopping server")
		close(closeServerSocket)
		<-serverSocketClosed
		fmt.Println("TEST server stopped")

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
				Port:  htons(4234),
				Addr1: binary.BigEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}),
			},
		)

		fmt.Println("checking server errors")

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		fmt.Println("checking client errors")
		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
		fmt.Println("done")
	})

	t.Run("client_sock_ipv6_tcp_connect_[::1]:5234_server_sock_ipv6_tcp_server_fin", func(t *testing.T) {
		if !nettest.SupportsIPv6() {
			t.Skip("IPv6 is not supported")
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

		fmt.Println("TEST Waiting for client to be connected and server to accept")
		<-clientConnected
		<-serverAccepted
		fmt.Println("TEST client connected and server accepted !")

		// retrieve client bound port
		clientPort := htons(uint16(<-clientBoundPort))

		// check client flow_pid entry
		fmt.Println("TEST checking client entry")
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
		fmt.Println("TEST checking server entry")
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

		// we can now close the server
		fmt.Println("TEST stopping server")
		close(closeServerSocket)
		<-serverSocketClosed
		fmt.Println("TEST server stopped")

		// check that no FlowPid entry leaked from the server side
		fmt.Println("TEST checking server side leak")
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

		// we can now close the client
		fmt.Println("TEST stopping client")
		close(closeClientSocket)
		<-clientSocketClosed
		fmt.Println("TEST client stopped")

		// client side
		fmt.Println("TEST checking client side leak")
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

		fmt.Println("checking server errors")

		// check for server errors
		err = <-serverErr
		for err != nil {
			t.Errorf("server error: %v", err)
			err = <-serverErr
		}

		fmt.Println("checking client errors")
		// check for client errors
		err = <-clientErr
		for err != nil {
			t.Errorf("client error: %v", err)
			err = <-clientErr
		}
		fmt.Println("done")
	})

	// - Revoir ce qui se passe lorsque le server binds 0.0.0.0 par rapport à l'accept created socket et les leaks de l'entrée en 0.0.0.0. Différencier close(serverFD) de close(connFD).
	// - Socket_closing ça ne devrait servir à rien. Tout est dans savoir différencier la socket du server VS connFD et savoir si ce sont les mêmes entrées ou non.
	// - Add tests when you bind before you connect ... I think the bind entry gets deleted which is good, but need to test.
}
