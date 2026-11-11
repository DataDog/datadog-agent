// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBoundedManagerSignalHandlerReportsSaturationWithoutBlocking(t *testing.T) {
	handler := newBoundedManagerSignalHandler()
	signals := make(chan *dbus.Signal, 1)
	handler.AddSignal(signals)
	first := &dbus.Signal{Name: "first"}
	handler.DeliverSignal("", "", first)

	delivered := make(chan struct{})
	go func() {
		handler.DeliverSignal("", "", &dbus.Signal{Name: "dropped"})
		close(delivered)
	}()
	select {
	case <-delivered:
	case <-time.After(time.Second):
		t.Fatal("saturated signal delivery blocked")
	}
	select {
	case <-handler.Overflow():
	default:
		t.Fatal("saturated signal delivery did not report overflow")
	}
	require.Len(t, signals, 1)
	assert.Same(t, first, <-signals)
}

func TestBoundedManagerSignalHandlerTerminateClosesRegisteredChannels(t *testing.T) {
	handler := newBoundedManagerSignalHandler()
	first := make(chan *dbus.Signal, 1)
	second := make(chan *dbus.Signal, 1)
	handler.AddSignal(first)
	handler.AddSignal(second)
	handler.AddSignal(first)
	handler.Terminate()
	handler.Terminate()
	handler.DeliverSignal("", "", &dbus.Signal{Name: "ignored"})

	_, firstOpen := <-first
	_, secondOpen := <-second
	assert.False(t, firstOpen)
	assert.False(t, secondOpen)
}

func TestBoundedManagerSignalHandlerRemovePreventsDeliveryAndClose(t *testing.T) {
	handler := newBoundedManagerSignalHandler()
	signals := make(chan *dbus.Signal, 1)
	handler.AddSignal(signals)
	handler.RemoveSignal(signals)
	handler.DeliverSignal("", "", &dbus.Signal{Name: "ignored"})
	handler.Terminate()

	assert.Empty(t, signals)
	close(signals)
}
