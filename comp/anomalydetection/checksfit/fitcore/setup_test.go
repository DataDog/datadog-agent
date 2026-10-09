// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Adapted from the Fast IPC Toolkit's lib/go/fitcore (module `fit`) at commit
// 4961722de9009afdbbb711fc0adf14a8f6ff9277 (ddoghq-sandbox/celian-26q4-innov-fast-ipc-toolkit).
//
// Local changes: the darwin build requires cgo, unsupported platforms get a
// stub so this tree still compiles, and test files carry an explicit platform
// gate. The transport logic, framing, and ring layout are unchanged.

//go:build linux || (darwin && cgo)

package fitcore

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testProtocol = ProtocolDescriptor{ID: array8("CORE0001"), Version: 2, MessageTypes: []uint32{1, 2}}

// testDirectory creates a private directory owned by the effective user for
// a pathname Unix socket.
func testDirectory(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("fit-core-%d-%x", os.Getpid(), testSessionID()))
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(dir) })
	return dir
}

func unusedLoopbackAddress(t *testing.T) netip.AddrPort {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).AddrPort()
}

func waitForSocket(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("consumer did not bind within two seconds")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestCancellationStopsWaitingForAPeer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := OpenConsumerContext(ctx, NewConsumerConfig(TCPEndpoint(unusedLoopbackAddress(t))), testProtocol)
		done <- err
	}()
	time.Sleep(30 * time.Millisecond)
	started := time.Now()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, ErrCancelled) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("open did not stop after cancellation")
	}
	if time.Since(started) > time.Second {
		t.Fatal("cancellation took too long")
	}
}

func TestCancellationStopsConnectRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := ConnectProducerContext(ctx, NewProducerConfig(TCPEndpoint(unusedLoopbackAddress(t))), testProtocol)
		done <- err
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, ErrCancelled) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("connect did not stop after cancellation")
	}
}

func TestCancellationStopsAPartialHello(t *testing.T) {
	addr := unusedLoopbackAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := OpenConsumerContext(ctx, NewConsumerConfig(TCPEndpoint(addr)), testProtocol)
		done <- err
	}()
	// Bind a raw peer and deliver a single byte of a Hello frame.
	deadline := time.Now().Add(2 * time.Second)
	var conn net.Conn
	var err error
	for conn == nil {
		conn, err = net.DialTimeout("tcp", addr.String(), 50*time.Millisecond)
		if err != nil {
			if time.Now().After(deadline) {
				t.Fatalf("consumer did not bind: %v", err)
			}
			time.Sleep(time.Millisecond)
		}
	}
	defer conn.Close()
	if _, err := conn.Write([]byte{0}); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, ErrCancelled) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("open did not stop after cancellation")
	}
}

func TestMismatchedProtocolIsRejectedDuringSetup(t *testing.T) {
	directory := testDirectory(t)
	socket := filepath.Join(directory, "setup.sock")
	done := make(chan error, 1)
	go func() {
		_, err := OpenConsumer(NewConsumerConfig(UnixEndpoint(socket)), testProtocol)
		done <- err
	}()
	waitForSocket(t, socket)
	other := ProtocolDescriptor{ID: array8("OTHER001"), Version: 2, MessageTypes: []uint32{1}}
	producerError := func() error {
		_, err := ConnectProducer(NewProducerConfig(UnixEndpoint(socket)), other)
		return err
	}()
	consumerError := <-done
	if producerError == nil || !strings.Contains(producerError.Error(), "mismatch") {
		t.Fatalf("producer error = %v", producerError)
	}
	if consumerError == nil || !strings.Contains(consumerError.Error(), "mismatch") {
		t.Fatalf("consumer error = %v", consumerError)
	}
	if _, err := os.Stat(socket); !os.IsNotExist(err) {
		t.Fatal("socket remained after a rejected handshake")
	}
}

func TestConfiguredListenerTimeoutIsHonored(t *testing.T) {
	directory := testDirectory(t)
	socket := filepath.Join(directory, "setup.sock")
	config := NewConsumerConfig(UnixEndpoint(socket))
	config.SetupTimeout = 40 * time.Millisecond
	start := time.Now()
	_, err := OpenConsumer(config, testProtocol)
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("timeout took too long")
	}
	if _, err := os.Stat(socket); !os.IsNotExist(err) {
		t.Fatal("socket remained after timeout")
	}
}

func TestLoopbackTCPSetupClosesListenerAndTransfersRecords(t *testing.T) {
	addr := unusedLoopbackAddress(t)
	type result struct {
		id      uint64
		kind    uint32
		payload []byte
		err     error
	}
	consumerDone := make(chan result, 1)
	go func() {
		consumer, err := OpenConsumer(NewConsumerConfig(TCPEndpoint(addr)), testProtocol)
		if err != nil {
			consumerDone <- result{err: err}
			return
		}
		defer consumer.Close()
		kind, payload, err := consumer.Receive()
		consumerDone <- result{id: consumer.SessionID(), kind: kind, payload: payload, err: err}
	}()
	// The connector retries refused connections until the deadline, so
	// independently started peers can race during listener startup.
	producer, err := ConnectProducer(NewProducerConfig(TCPEndpoint(addr)), testProtocol)
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	payload := []byte("tcp setup uses the same shared queue")
	sent, err := producer.SendBatch([]Record{{Kind: 1, Payload: payload}})
	if err != nil || sent.Accepted != 1 {
		t.Fatalf("send = %+v, %v", sent, err)
	}
	received := <-consumerDone
	if received.err != nil {
		t.Fatal(received.err)
	}
	if producer.SessionID() != received.id {
		t.Fatalf("session ids differ: %x vs %x", producer.SessionID(), received.id)
	}
	if received.kind != 1 || !bytes.Equal(received.payload, payload) {
		t.Fatalf("received %d %q", received.kind, received.payload)
	}
	conn, err := net.DialTimeout("tcp", addr.String(), 50*time.Millisecond)
	if err == nil {
		conn.Close()
		t.Fatal("TCP listener remained after setup")
	}
}

func TestTCPSetupRejectsNonLoopbackAddresses(t *testing.T) {
	config := NewConsumerConfig(TCPEndpoint(netip.MustParseAddrPort("192.0.2.1:5101")))
	_, err := OpenConsumer(config, testProtocol)
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("error = %v", err)
	}
}

// TestEOFBeforeStartIsSetupFailure drives a fake consumer that completes the
// handshake through Ready and then closes, which must never be interpreted
// as Start.
func TestEOFBeforeStartIsSetupFailure(t *testing.T) {
	directory := testDirectory(t)
	socket := filepath.Join(directory, "setup.sock")
	id := testSessionID()
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		listener.Close()
		os.Remove(socket)
	}()
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		deadline := time.Now().Add(DefaultSetupTimeout)
		hello, err := receiveFrame(conn, deadline, nil)
		if err != nil {
			return
		}
		body, err := expectedMessage(hello, 1)
		if err != nil {
			return
		}
		if err := checkContract(body, 1, testProtocol); err != nil {
			return
		}
		shared, name, err := createMapping(id, DefaultRingCapacity, testProtocol.Version)
		if err != nil {
			return
		}
		defer shared.close()
		if err := sendFrame(conn, offerFrame(id, name, DefaultRingCapacity, testProtocol), deadline, nil); err != nil {
			return
		}
		ready, err := receiveFrame(conn, deadline, nil)
		if err != nil {
			return
		}
		if body, err := expectedMessage(ready, 4); err != nil || !bytes.Equal(body, be64(id)) {
			return
		}
		// Closing here must never be interpreted as Start.
	}()
	producerError := func() error {
		_, err := ConnectProducer(NewProducerConfig(UnixEndpoint(socket)), testProtocol)
		return err
	}()
	<-serverDone
	if producerError == nil ||
		!errors.Is(producerError, io.ErrUnexpectedEOF) ||
		!strings.Contains(producerError.Error(), "waiting for Start") {
		t.Fatalf("producer error = %v", producerError)
	}
}

// TestGoConsumerAcceptsRustProducerHello verifies the Hello/Offer wire bytes
// against a raw peer acting exactly like the Rust template's producer.
func TestSetupFramesMatchWireContract(t *testing.T) {
	directory := testDirectory(t)
	socket := filepath.Join(directory, "setup.sock")
	consumerDone := make(chan error, 1)
	go func() {
		consumer, err := OpenConsumer(NewConsumerConfig(UnixEndpoint(socket)), testProtocol)
		if err != nil {
			consumerDone <- err
			return
		}
		consumer.Close()
		consumerDone <- nil
	}()
	waitForSocket(t, socket)
	conn, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	// Hello: tag 1 plus the 29-byte producer contract tuple.
	if err := sendFrame(conn, append([]byte{1}, contractBytes(1, testProtocol)...), time.Now().Add(5*time.Second), nil); err != nil {
		t.Fatal(err)
	}
	offer, err := receiveFrame(conn, time.Now().Add(5*time.Second), nil)
	if err != nil {
		t.Fatal(err)
	}
	body, err := expectedMessage(offer, 3)
	if err != nil {
		t.Fatal(err)
	}
	contractLen := len(contractBytes(2, testProtocol))
	if err := checkContract(body[:contractLen], 2, testProtocol); err != nil {
		t.Fatal(err)
	}
	regionSize := ringOffset + DefaultRingCapacity
	tail := body[contractLen:]
	id := binary.BigEndian.Uint64(tail[:8])
	if got := binary.BigEndian.Uint32(tail[8:12]); got != uint32(regionSize) {
		t.Fatalf("region size = %d", got)
	}
	if got := binary.BigEndian.Uint32(tail[12:16]); got != ringOffset {
		t.Fatalf("ring offset = %d", got)
	}
	if got := binary.BigEndian.Uint32(tail[16:20]); got != DefaultRingCapacity {
		t.Fatalf("ring capacity = %d", got)
	}
	if got := binary.BigEndian.Uint32(tail[20:24]); got != recordHeaderSize {
		t.Fatalf("record header = %d", got)
	}
	nameLen := int(tail[24])
	if len(tail) != 25+nameLen {
		t.Fatalf("name length %d does not match body", nameLen)
	}
	name := string(tail[25:])
	if !strings.HasPrefix(name, "/mc-") || len(name) > 30 || strings.Contains(name[1:], "/") {
		t.Fatalf("shm name %q is invalid", name)
	}
	// Ready with the offered session id, then wait for Start.
	ready := append([]byte{4}, be64(id)...)
	if err := sendFrame(conn, ready, time.Now().Add(5*time.Second), nil); err != nil {
		t.Fatal(err)
	}
	start, err := receiveFrame(conn, time.Now().Add(5*time.Second), nil)
	if err != nil {
		t.Fatal(err)
	}
	startBody, err := expectedMessage(start, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(startBody, be64(id)) {
		t.Fatal("Start session identifier mismatch")
	}
	if err := <-consumerDone; err != nil {
		t.Fatalf("consumer error: %v", err)
	}
	if _, err := os.Stat(socket); !os.IsNotExist(err) {
		t.Fatal("socket remained after setup")
	}
}
