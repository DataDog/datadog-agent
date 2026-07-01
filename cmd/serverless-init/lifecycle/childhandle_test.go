// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package lifecycle

import (
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChild_StartsNotAlive(t *testing.T) {
	assert.False(t, NewChild().IsAlive())
}

// TestNewChild_InitializesAtomicFields pins the current contract: alive and
// proc are *atomic.Bool/*atomic.Pointer (go.uber.org/atomic, required for
// struct-field alignment safety), so a bare Child{} would leave them nil —
// NewChild must initialize both.
func TestNewChild_InitializesAtomicFields(t *testing.T) {
	c := NewChild()
	assert.NotNil(t, c.alive, "NewChild must initialize alive — a bare Child{} would panic on first use")
	assert.NotNil(t, c.proc, "NewChild must initialize proc — a bare Child{} would panic on first use")
}

func TestChild_MarkAliveSetsAlive(t *testing.T) {
	h := NewChild()
	h.MarkAlive()
	assert.True(t, h.IsAlive())
}

func TestChild_MarkDeadClearsAlive(t *testing.T) {
	h := NewChild()
	h.MarkAlive()
	h.MarkDead()
	assert.False(t, h.IsAlive())
}

func TestNoopChildHandle_AlwaysNotAlive(t *testing.T) {
	assert.False(t, NewNoopChildHandle().IsAlive())
}

func TestRunSignal_IsSIGUSR2(t *testing.T) {
	assert.Equal(t, syscall.SIGUSR2, RunSignal)
}

func TestChild_SignalRun_NilProcess_ReturnsNil(t *testing.T) {
	c := NewChild()
	assert.NoError(t, c.SignalRun(syscall.SIGUSR2))
}

func TestChild_StoreProcess_SignalRun_DeliversSignal(t *testing.T) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGUSR2)
	defer signal.Stop(sigCh)

	c := NewChild()
	self, err := os.FindProcess(os.Getpid())
	require.NoError(t, err)
	c.StoreProcess(self)

	require.NoError(t, c.SignalRun(syscall.SIGUSR2))

	select {
	case sig := <-sigCh:
		assert.Equal(t, syscall.SIGUSR2, sig)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("SIGUSR2 not received within 500ms after SignalRun")
	}
}
