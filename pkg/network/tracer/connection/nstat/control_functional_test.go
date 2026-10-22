// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package nstat

import (
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

const functionalDatagramBufferSize = 65535

func TestControlFunctionalRevision9Stream(t *testing.T) {
	if os.Getenv("RUN_NSTAT_FUNCTIONAL_TEST") != "1" {
		t.Skip("set RUN_NSTAT_FUNCTIONAL_TEST=1 to exercise the private kernel control")
	}

	control, err := OpenControl()
	require.NoError(t, err)
	defer control.Close()
	_, err = control.Subscribe(ProviderTCPKernel)
	require.NoError(t, err)
	_, err = control.Subscribe(ProviderTCPUserland)
	require.NoError(t, err)

	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer listener.Close()

	client, err := net.DialTCP("tcp4", nil, listener.Addr().(*net.TCPAddr))
	require.NoError(t, err)
	defer client.Close()
	server, err := listener.AcceptTCP()
	require.NoError(t, err)
	defer server.Close()
	_, err = client.Write([]byte("nstat-client"))
	require.NoError(t, err)
	payload := make([]byte, len("nstat-client"))
	_, err = io.ReadFull(server, payload)
	require.NoError(t, err)
	_, err = server.Write([]byte("nstat-server"))
	require.NoError(t, err)
	payload = make([]byte, len("nstat-server"))
	_, err = io.ReadFull(client, payload)
	require.NoError(t, err)

	clientLocal := client.LocalAddr().(*net.TCPAddr).AddrPort()
	clientRemote := client.RemoteAddr().(*net.TCPAddr).AddrPort()
	require.NoError(t, control.QueryAll())

	buffer := make([]byte, functionalDatagramBufferSize)
	deadline := time.Now().Add(10 * time.Second)
	eventCounts := make(map[EventKind]int)
	providerCounts := make(map[uint32]int)
	pidCounts := make(map[uint32]int)
	errorCounts := make(map[uint32]int)
	var descriptionQueue []uint64
	nextDescription := time.Now()
	for time.Now().Before(deadline) {
		pollTimeout := min(20*time.Millisecond, time.Until(deadline))
		ready, err := control.Poll(pollTimeout)
		require.NoError(t, err)
		if !ready {
			if len(descriptionQueue) > 0 && !time.Now().Before(nextDescription) {
				require.NoError(t, control.RequestDescription(descriptionQueue[0]))
				descriptionQueue = descriptionQueue[1:]
				nextDescription = time.Now().Add(20 * time.Millisecond)
			}
			continue
		}
		for {
			n, err := control.Receive(buffer)
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
				break
			}
			require.NoError(t, err)
			events, err := ParseDatagram(buffer[:n])
			require.NoError(t, err)
			for _, event := range events {
				eventCounts[event.Kind]++
				if event.Provider != 0 {
					providerCounts[event.Provider]++
				}
				if event.Kind == EventError {
					errorCounts[event.Error]++
				}
				if event.Kind == EventAdded {
					descriptionQueue = append(descriptionQueue, event.SourceRef)
				}
				if event.Flow != nil {
					pidCounts[event.Flow.PID]++
					require.Less(t, event.Flow.PID, uint32(1<<30))
					if IsTCPProvider(event.Provider) &&
						event.Flow.PID == uint32(os.Getpid()) &&
						event.Flow.Local.Address == clientLocal.Addr() &&
						event.Flow.Local.Port == clientLocal.Port() &&
						event.Flow.Remote.Address == clientRemote.Addr() &&
						event.Flow.Remote.Port == clientRemote.Port() {
						return
					}
				}
			}
		}
		if len(descriptionQueue) > 0 && !time.Now().Before(nextDescription) {
			require.NoError(t, control.RequestDescription(descriptionQueue[0]))
			descriptionQueue = descriptionQueue[1:]
			nextDescription = time.Now().Add(20 * time.Millisecond)
		}
	}
	t.Fatalf(
		"NStat did not return a revision-9 descriptor while loopback client %s -> %s for PID %d was active; events=%v providers=%v errors=%v pids=%v",
		clientLocal,
		clientRemote,
		os.Getpid(),
		eventCounts,
		providerCounts,
		errorCounts,
		pidCounts,
	)
}

func TestControlFunctionalRevision9UDP(t *testing.T) {
	if os.Getenv("RUN_NSTAT_FUNCTIONAL_TEST") != "1" {
		t.Skip("set RUN_NSTAT_FUNCTIONAL_TEST=1 to exercise the private kernel control")
	}

	control, err := OpenControl()
	require.NoError(t, err)
	defer control.Close()
	_, err = control.Subscribe(ProviderUDPKernel)
	require.NoError(t, err)
	_, err = control.Subscribe(ProviderUDPUserland)
	require.NoError(t, err)

	server, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer server.Close()
	client, err := net.DialUDP("udp4", nil, server.LocalAddr().(*net.UDPAddr))
	require.NoError(t, err)
	defer client.Close()
	_, err = client.Write([]byte("nstat-udp"))
	require.NoError(t, err)
	payload := make([]byte, len("nstat-udp"))
	_, _, err = server.ReadFromUDP(payload)
	require.NoError(t, err)
	require.NoError(t, control.QueryAll())

	clientPort := uint16(client.LocalAddr().(*net.UDPAddr).Port)
	serverPort := uint16(server.LocalAddr().(*net.UDPAddr).Port)
	buffer := make([]byte, functionalDatagramBufferSize)
	deadline := time.Now().Add(5 * time.Second)
	eventCounts := make(map[EventKind]int)
	providerCounts := make(map[uint32]int)
	errorCounts := make(map[uint32]int)
	var parserErrors []error
	var observedFlows []Flow
	var descriptionQueue []uint64
	nextDescription := time.Now()
	for time.Now().Before(deadline) {
		pollTimeout := min(20*time.Millisecond, time.Until(deadline))
		ready, err := control.Poll(pollTimeout)
		require.NoError(t, err)
		if !ready {
			if len(descriptionQueue) > 0 && !time.Now().Before(nextDescription) {
				require.NoError(t, control.RequestDescription(descriptionQueue[0]))
				descriptionQueue = descriptionQueue[1:]
				nextDescription = time.Now().Add(20 * time.Millisecond)
			}
			continue
		}
		for {
			n, err := control.Receive(buffer)
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
				break
			}
			require.NoError(t, err)
			events, err := ParseDatagram(buffer[:n])
			if err != nil {
				parserErrors = append(parserErrors, err)
				continue
			}
			for _, event := range events {
				eventCounts[event.Kind]++
				if event.Provider != 0 {
					providerCounts[event.Provider]++
				}
				if event.Kind == EventError {
					errorCounts[event.Error]++
				}
				if event.Kind == EventAdded {
					descriptionQueue = append(descriptionQueue, event.SourceRef)
				}
				if event.Flow != nil &&
					IsUDPProvider(event.Provider) {
					if len(observedFlows) < 100 {
						observedFlows = append(observedFlows, *event.Flow)
					}
					if event.Flow.PID == uint32(os.Getpid()) &&
						event.Flow.Local.Port == clientPort &&
						event.Flow.Remote.Port == serverPort {
						return
					}
				}
			}
		}
		if len(descriptionQueue) > 0 && !time.Now().Before(nextDescription) {
			require.NoError(t, control.RequestDescription(descriptionQueue[0]))
			descriptionQueue = descriptionQueue[1:]
			nextDescription = time.Now().Add(20 * time.Millisecond)
		}
	}
	t.Fatalf(
		"NStat did not return UDP descriptor %d -> %d for PID %d; events=%v providers=%v errors=%v parser_errors=%v flows=%+v",
		clientPort,
		serverPort,
		os.Getpid(),
		eventCounts,
		providerCounts,
		errorCounts,
		parserErrors,
		observedFlows,
	)
}
