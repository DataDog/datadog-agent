// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package netlink

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/netlink/testutil"
	nettestutil "github.com/DataDog/datadog-agent/pkg/network/testutil"
)

func TestConsumerKeepsRunningAfterCircuitBreakerTrip(t *testing.T) {
	ns := testutil.SetupCrossNsDNAT(t)
	cfg := &config.Config{
		Config: ebpf.Config{
			ProcRoot: "/proc",
		},
		ConntrackRateLimit:           1,
		ConntrackRateLimitInterval:   500 * time.Millisecond,
		EnableRootNetNs:              true,
		EnableConntrackAllNamespaces: false,
	}
	c, err := NewConsumer(cfg, nil)
	require.NoError(t, err)
	require.NotNil(t, c)
	exited := make(chan struct{})
	defer func() {
		c.Stop()
		<-exited
	}()

	ev, err := c.Events()
	require.NoError(t, err)
	require.NotNil(t, ev)

	go func() {
		defer close(exited)
	}()

	isRecvLoopRunning := c.recvLoopRunning.Load
	require.Eventually(t, isRecvLoopRunning, cfg.ConntrackRateLimitInterval*2, 100*time.Millisecond)

	srv := nettestutil.StartServerTCPNs(t, net.ParseIP("2.2.2.4"), 0, ns)
	defer srv.Close()

	l := srv.(net.Listener)

	// this should trip the circuit breaker
	// we have to run this loop for more than
	// `tickInterval` seconds (3s currently) for
	// the circuit breaker to detect the over-limit
	// rate of updates
	sleepAmt := 50 * time.Millisecond
	loopTime := 2 * cfg.ConntrackRateLimitInterval
	loopCount := loopTime.Nanoseconds() / sleepAmt.Nanoseconds()

	for i := int64(0); i < loopCount; i++ {
		conn, err := net.Dial("tcp", l.Addr().String())
		require.NoError(t, err)
		defer conn.Close()
		time.Sleep(sleepAmt)
	}

	// on pre 3.15 kernels, the receive loop
	// will simply bail since bpf random sampling
	// is not available
	if pre315Kernel {
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			assert.False(collect, isRecvLoopRunning(), "receive loop should not be running")
			assert.True(collect, c.breaker.IsOpen(), "breaker should be open")
		}, 2*time.Second, 100*time.Millisecond)
	} else {
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			assert.Lessf(collect, c.samplingRate, 1.0, "sampling rate should be less than 1.0")
		}, 2*time.Second, 100*time.Millisecond)
		require.True(t, isRecvLoopRunning())
	}
}
