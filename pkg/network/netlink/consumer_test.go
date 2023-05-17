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
		ConntrackRateLimitInterval:   1 * time.Second,
		EnableRootNetNs:              true,
		EnableConntrackAllNamespaces: false,
	}
	c, err := NewConsumer(cfg)
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
		for range ev {
		}
	}()

	isRecvLoopRunning := c.recvLoopRunning.Load
	require.Eventually(t, isRecvLoopRunning, cfg.ConntrackRateLimitInterval, 100*time.Millisecond)

	srv := nettestutil.StartServerTCPNs(t, net.ParseIP("2.2.2.4"), 0, ns)
	defer srv.Close()

	l := srv.(net.Listener)

	// this should trip the circuit breaker
	// we have to run this loop for more than
	// `tickInterval` seconds (3s currently) for
	// the circuit breaker to detect the over-limit
	// rate of updates
	sleepAmt := 250 * time.Millisecond
	loopCount := (cfg.ConntrackRateLimitInterval.Nanoseconds() / sleepAmt.Nanoseconds()) + 1

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
		require.Eventually(t, func() bool {
			return !isRecvLoopRunning() && c.breaker.IsOpen()
		}, cfg.ConntrackRateLimitInterval, 100*time.Millisecond)
	} else {
		require.Eventually(t, func() bool {
			return c.samplingRate < 1.0
		}, cfg.ConntrackRateLimitInterval, 100*time.Millisecond)
		require.True(t, isRecvLoopRunning())
	}
}
