// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux_bpf

package probe

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestTCPQueueLengthTracer(t *testing.T) {
	kv, err := kernel.HostVersion()
	if err != nil {
		t.Fatal(err)
	}
	if kv < kernel.VersionCode(4, 8, 0) {
		t.Skipf("Kernel version %v is not supported by the OOM probe", kv)
	}

	cfg := ebpf.NewConfig()

	tcpTracer, err := NewTCPQueueLengthTracer(cfg)
	if err != nil {
		t.Fatal(err)
	}

	beforeStats := extractGlobalStats(t, tcpTracer)
	if beforeStats.ReadBufferMaxUsage > 10 {
		t.Error("max usage of read buffer is too big before the stress test")
	}

	runTCPLoadTest()

	afterStats := extractGlobalStats(t, tcpTracer)
	if afterStats.ReadBufferMaxUsage < 1000 {
		t.Error("max usage of read buffer is too low after the stress test")
	}

	defer tcpTracer.Close()
}

func extractGlobalStats(t *testing.T, tracer *TCPQueueLengthTracer) TCPQueueLengthStatsValue {
	t.Helper()

	stats := tracer.GetAndFlush()
	if stats == nil {
		t.Error("failed to get and flush stats")
	}

	globalStats, ok := stats[""]
	if !ok {
		return TCPQueueLengthStatsValue{}
	}

	return globalStats
}

// TCP test infrastructure
// The idea here is to setup a server and a client, and to slow the server as much as possible by:
// - reading slowly (wait between reads)
// - reading small chunks at a time
// - reducing the RECV buffer size

var Addr *net.TCPAddr = &net.TCPAddr{
	Port: 25568,
}

var (
	isInSlowMode    = true
	wg              sync.WaitGroup
	serverReadyLock sync.Mutex
	serverReadyCond = sync.NewCond(&serverReadyLock)
)

func handleRequest(conn *net.TCPConn) error {
	defer wg.Done()
	total := 0
outer:
	for {
		buf := make([]byte, 10)
		count, err := conn.Read(buf)
		if err != nil {
			return err
		}

		total += count

		for i := 0; i < count; i++ {
			if buf[i] == 0 {
				break outer
			}
		}

		if isInSlowMode {
			time.Sleep(1 * time.Second)
		}
	}

	conn.Close()
	return nil
}

func server() error {
	listener, err := net.ListenTCP("tcp", Addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	serverReadyCond.Broadcast()

	conn, err := listener.AcceptTCP()

	if err != nil {
		return err
	}
	conn.SetReadBuffer(2)

	return handleRequest(conn)
}

const MSG_LEN = 10000

func client() error {
	defer wg.Done()

	serverReadyCond.Wait()

	conn, err := net.DialTCP("tcp", nil, Addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	msg := make([]byte, MSG_LEN)
	for i := 0; i < MSG_LEN-1; i++ {
		msg[i] = 4
	}
	msg[MSG_LEN-1] = 0

	conn.Write(msg)

	isInSlowMode = false
	return nil
}

func runTCPLoadTest() {
	serverReadyLock.Lock()

	wg.Add(2)
	go server()
	go client()
	wg.Wait()
}
