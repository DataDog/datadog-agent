// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package http

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/network/driver"
)

// shutdownTimeout bounds the waits below so a regression fails the test
// instead of hanging it. It is not a measurement: the operations under test
// complete immediately when they are correct.
const shutdownTimeout = 30 * time.Second

// endlessTransactionHandle is a driver handle whose transaction buffer never
// runs dry, which is what a host under sustained HTTP traffic looks like:
// every flush hands back another batch.
type endlessTransactionHandle struct{}

func (*endlessTransactionHandle) SynchronousDeviceIoControl(_ uint32, _ *byte, _ uint32, _ *byte, outBufferSize uint32) (uint32, error) {
	if outBufferSize < uint32(driver.HttpTransactionTypeSize) {
		return 0, nil
	}
	// The caller zeroes the buffer, so a header-sized read parses as one
	// transaction carrying no request fragment.
	return uint32(driver.HttpTransactionTypeSize), nil
}

func (*endlessTransactionHandle) ReadFile(_ []byte, _ *uint32, _ *windows.Overlapped) error {
	return nil
}

func (*endlessTransactionHandle) DeviceIoControl(_ uint32, _ *byte, _ uint32, _ *byte, _ uint32, _ *uint32, _ *windows.Overlapped) error {
	return nil
}

func (*endlessTransactionHandle) CancelIoEx(_ *windows.Overlapped) error { return nil }

func (*endlessTransactionHandle) Close() error { return nil }

func (*endlessTransactionHandle) GetWindowsHandle() windows.Handle { return windows.Handle(0) }

func (*endlessTransactionHandle) RefreshStats() {}

func newTestDriverInterface() *HttpDriverInterface {
	return &HttpDriverInterface{
		driverHTTPHandle: &endlessTransactionHandle{},
		// One transaction per flush, with no request fragment, is the
		// smallest buffer readPendingTransactions accepts.
		maxTransactions:    1,
		maxRequestFragment: 0,
		DataChannel:        make(chan []WinHttpTransaction),
	}
}

func waitFor(t *testing.T, done <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(shutdownTimeout):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// TestReadAllPendingTransactionsGivesUpWhenClosing covers a drain that would
// otherwise run for as long as transactions keep arriving. Shutdown has to be
// able to cut it short, whoever started it.
func TestReadAllPendingTransactionsGivesUpWhenClosing(t *testing.T) {
	di := newTestDriverInterface()

	drained := make(chan struct{})
	go func() {
		defer close(drained)
		di.ReadAllPendingTransactions()
	}()

	// Take a few batches first so the drain is demonstrably under way, rather
	// than testing the check on entry.
	for i := 0; i < 3; i++ {
		<-di.DataChannel
	}
	di.PrepareStop()

	// Keep receiving: the drain must be free to finish the batch it is on.
	deadline := time.After(shutdownTimeout)
	for {
		select {
		case <-drained:
			return
		case <-di.DataChannel:
		case <-deadline:
			t.Fatal("ReadAllPendingTransactions kept draining after PrepareStop")
		}
	}
}

// TestCloseInterruptsDrainInProgress covers the shutdown path: Close has to
// close DataChannel without waiting for the driver to run dry, and without
// racing the send of a drain that is already in progress.
func TestCloseInterruptsDrainInProgress(t *testing.T) {
	eventHandle, err := windows.CreateEvent(nil, 1, 0, nil)
	require.NoError(t, err)

	di := newTestDriverInterface()
	di.driverEventHandle = eventHandle

	// Stands in for the monitor event loop, so the drain is never blocked on a
	// send. A send after Close would panic here rather than deadlock.
	firstBatch := make(chan struct{})
	consumed := make(chan struct{})
	go func() {
		defer close(consumed)
		var once sync.Once
		for range di.DataChannel {
			once.Do(func() { close(firstBatch) })
		}
	}()

	drained := make(chan struct{})
	go func() {
		defer close(drained)
		di.ReadAllPendingTransactions()
	}()

	waitFor(t, firstBatch, "the drain to start producing")

	closed := make(chan error, 1)
	go func() {
		closed <- di.Close()
	}()

	select {
	case err := <-closed:
		require.NoError(t, err)
	case <-time.After(shutdownTimeout):
		t.Fatal("Close blocked while transactions kept arriving")
	}
	waitFor(t, drained, "the drain to return")
	waitFor(t, consumed, "DataChannel to be closed")
}
